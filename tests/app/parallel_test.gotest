// Package app tests optional parallel helper gates around sealed runs.
package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAdvanceLetsCleanReviewNormalizeParallelGateInputFinding verifies raw helper findings do not override the reviewer.
func TestAdvanceLetsCleanReviewNormalizeParallelGateInputFinding(t *testing.T) {
	// Given: a review stage with parallel review enabled and a clean review artifact.
	repo := gitRepo(t)
	runID := "parallel-review-gate-run"
	base := runDir(repo, runID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWritePrompt(t, filepath.Join(base, "review-1.json"), cleanReviewJSON())
	mustWritePrompt(t, filepath.Join(base, "parallel-review-1.json"), parallelReviewArtifactForTest(parallelMemberFixture{
		name:    "代码质量审核员",
		purpose: "check implementation quality",
		status:  "success",
		summary: "found ignored runtime issue",
		extra:   `"findings":[{"title":"runtime path broken","severity":"major","evidence":"parallel review trace failed","recommendation":"fix runtime path before QA"}]`,
	}))
	state := State{RunID: runID, Stage: "review_1", Status: statusRunning, Workflow: parallelWorkflowForTest()}

	// When: the state machine advances the review stage.
	err := (&Engine{Repo: repo}).advance(&state)

	// Then: the main reviewer's clean artifact advances; raw helper findings are advisory input.
	if err != nil {
		t.Fatalf("advance should accept normalized clean review, got %v", err)
	}
	if state.Stage != "qa_1" {
		t.Fatalf("stage should advance to QA after clean review, got %s", state.Stage)
	}
}

// TestAdvanceLetsCleanReviewNormalizeParallelRequiredMemberFailure verifies member status is reviewer input.
func TestAdvanceLetsCleanReviewNormalizeParallelRequiredMemberFailure(t *testing.T) {
	// Given: a review stage with a clean review artifact and a failed required helper.
	repo := gitRepo(t)
	runID := "parallel-review-required-gate-run"
	base := runDir(repo, runID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWritePrompt(t, filepath.Join(base, "review-1.json"), cleanReviewJSON())
	mustWritePrompt(t, filepath.Join(base, "parallel-review-1.json"), parallelReviewArtifactForTest(parallelMemberFixture{
		name:    "目标核对审核员",
		purpose: "verify target scope",
		status:  "failed",
		summary: "could not complete target check",
		extra:   `"required":true,"evidence":["test-results/parallel-review-target.log"]`,
	}))
	state := State{RunID: runID, Stage: "review_1", Status: statusRunning, Workflow: parallelWorkflowForTest()}

	// When: the state machine advances the review stage.
	err := (&Engine{Repo: repo}).advance(&state)

	// Then: QA can proceed because the main review owns the normalized decision.
	if err != nil {
		t.Fatalf("advance should accept normalized clean review despite raw helper failure, got %v", err)
	}
	if state.Stage != "qa_1" {
		t.Fatalf("stage should advance to QA after clean review, got %s", state.Stage)
	}
}

// TestAdvanceBlocksCleanReviewWhenParallelMembersIncomplete verifies all configured reviewers must report.
func TestAdvanceBlocksCleanReviewWhenParallelMembersIncomplete(t *testing.T) {
	// Given: a clean review with only one successful parallel reviewer result.
	repo := gitRepo(t)
	runID := "parallel-review-incomplete-run"
	base := runDir(repo, runID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWritePrompt(t, filepath.Join(base, "review-1.json"), cleanReviewJSON())
	mustWritePrompt(t, filepath.Join(base, "parallel-review-1.json"), `{
		"group":"review",
		"mode":"gate_input",
		"summary":"only one reviewer reported",
		"members":[{
			"name":"代码质量审核员",
			"purpose":"verify target scope",
			"status":"success",
			"summary":"target scope passed"
		}]
	}`)
	state := State{RunID: runID, Stage: "review_1", Status: statusRunning, Workflow: parallelWorkflowForTest()}

	// When: the state machine advances the review stage.
	err := (&Engine{Repo: repo}).advance(&state)

	// Then: QA remains blocked because configured reviewer members are missing.
	if err == nil || !strings.Contains(err.Error(), "缺少配置成员") {
		t.Fatalf("advance should reject incomplete parallel review coverage, got %v", err)
	}
	if state.Stage != "review_1" {
		t.Fatalf("stage advanced despite incomplete parallel review coverage: %s", state.Stage)
	}
}

// TestAdvanceAllowsCleanReviewWhenParallelMembersComplete verifies complete successful review input may pass.
func TestAdvanceAllowsCleanReviewWhenParallelMembersComplete(t *testing.T) {
	// Given: clean review and all configured parallel reviewers reported success.
	repo := gitRepo(t)
	runID := "parallel-review-complete-run"
	base := runDir(repo, runID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWritePrompt(t, filepath.Join(base, "review-1.json"), cleanReviewJSON())
	mustWritePrompt(t, filepath.Join(base, "parallel-review-1.json"), parallelReviewArtifactForTest())
	state := State{RunID: runID, Stage: "review_1", Status: statusRunning, Workflow: parallelWorkflowForTest()}

	// When: the state machine advances the review stage.
	err := (&Engine{Repo: repo}).advance(&state)

	// Then: QA is the next gate.
	if err != nil {
		t.Fatalf("advance should allow complete clean parallel review, got %v", err)
	}
	if state.Stage != "qa_1" {
		t.Fatalf("stage = %s, want qa_1", state.Stage)
	}
}

// TestArtifactDoneRequiresParallelImplementationContextWhenEnabled verifies execution cannot skip context artifacts.
func TestArtifactDoneRequiresParallelImplementationContextWhenEnabled(t *testing.T) {
	// Given: an execution stage whose tasks are done but implementation context is missing.
	repo := gitRepo(t)
	runID := "parallel-context-missing-run"
	base := runDir(repo, runID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	writeDoneTask(t, repo)
	mustWritePrompt(t, filepath.Join(base, "parallel-planning-context.json"), parallelPlanningArtifactForTest())
	state := State{RunID: runID, ChangeName: "demo", Stage: "execution", Status: statusRunning, Workflow: parallelWorkflowForTest()}

	// When: the state machine checks whether execution is complete.
	done, err := (&Engine{Repo: repo}).artifactDone(state)

	// Then: completion is rejected until parallel-implementation-context.json is present.
	if err == nil || !strings.Contains(err.Error(), "parallel-implementation-context.json") {
		t.Fatalf("artifactDone should require implementation context, done=%v err=%v", done, err)
	}
}

// TestArtifactDoneAllowsAdvisoryImplementationContextFailure verifies non-required context failures are prompt input.
func TestArtifactDoneAllowsAdvisoryImplementationContextFailure(t *testing.T) {
	// Given: execution has finished and a non-required advisory context member failed.
	repo := gitRepo(t)
	runID := "parallel-context-advisory-run"
	base := runDir(repo, runID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	writeDoneTask(t, repo)
	mustWritePrompt(t, filepath.Join(base, "parallel-planning-context.json"), parallelPlanningArtifactForTest())
	mustWritePrompt(t, filepath.Join(base, "parallel-implementation-context.json"), parallelContextArtifactForTest(parallelMemberFixture{
		name:    "外部资料研究员",
		purpose: "query external docs",
		status:  "failed",
		summary: "network unavailable",
	}))
	state := State{RunID: runID, ChangeName: "demo", Stage: "execution", Status: statusRunning, Workflow: parallelWorkflowForTest()}

	// When: the state machine checks execution completion.
	done, err := (&Engine{Repo: repo}).artifactDone(state)

	// Then: advisory failure is accepted as recorded context for the main stage.
	if err != nil || !done {
		t.Fatalf("artifactDone should allow non-required advisory failure, done=%v err=%v", done, err)
	}
}

// TestArtifactDoneBlocksRequiredImplementationContextFailure verifies required context members are hard gates.
func TestArtifactDoneBlocksRequiredImplementationContextFailure(t *testing.T) {
	// Given: execution has finished but a required implementation context member failed.
	repo := gitRepo(t)
	runID := "parallel-context-required-run"
	base := runDir(repo, runID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	writeDoneTask(t, repo)
	mustWritePrompt(t, filepath.Join(base, "parallel-planning-context.json"), parallelPlanningArtifactForTest())
	mustWritePrompt(t, filepath.Join(base, "parallel-implementation-context.json"), parallelContextArtifactForTest(parallelMemberFixture{
		name:    "代码库侦察员",
		purpose: "collect implementation files",
		status:  "failed",
		summary: "could not inspect repository",
		extra:   `"required":true`,
	}))
	state := State{RunID: runID, ChangeName: "demo", Stage: "execution", Status: statusRunning, Workflow: parallelWorkflowForTest()}

	// When: the state machine checks execution completion.
	done, err := (&Engine{Repo: repo}).artifactDone(state)

	// Then: execution cannot complete with failed required context.
	if err == nil || !strings.Contains(err.Error(), "parallel-implementation-context.json") {
		t.Fatalf("artifactDone should reject required implementation context failure, done=%v err=%v", done, err)
	}
}

// TestAdvanceBlocksCleanQAWhenParallelRequiredMemberFails verifies QA clean cannot ignore required helper failures.
func TestAdvanceBlocksCleanQAWhenParallelRequiredMemberFails(t *testing.T) {
	// Given: a QA stage with complete acceptance coverage but a failed required helper.
	repo := gitRepo(t)
	runID := "parallel-qa-gate-run"
	base := runDir(repo, runID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWritePrompt(t, filepath.Join(base, "acceptance.json"), acceptanceJSON())
	mustWritePrompt(t, filepath.Join(base, "qa-1.json"), cleanQAJSON())
	mustWritePrompt(t, filepath.Join(base, "parallel-qa-1.json"), parallelQAArtifactForTest(parallelMemberFixture{
		name:    "CLI/API 测试员",
		purpose: "exercise the real command path",
		status:  "failed",
		summary: "command returned a failing exit code",
		extra:   `"required":true,"evidence":["test-results/parallel-cli.log"]`,
	}))
	state := State{RunID: runID, ChangeName: "demo", Stage: "qa_1", Status: statusRunning, Workflow: parallelWorkflowForTest()}

	// When: the state machine advances the QA stage.
	err := (&Engine{Repo: repo}).advance(&state)

	// Then: archive remains blocked because clean QA ignored a required helper failure.
	if err == nil || !strings.Contains(err.Error(), "parallel-qa-1.json") {
		t.Fatalf("advance should reject ignored parallel QA failure, got %v", err)
	}
	if state.Stage != "qa_1" {
		t.Fatalf("stage advanced despite parallel QA gate: %s", state.Stage)
	}
}

// TestAdvanceBlocksCleanQAWhenParallelMembersIncomplete verifies all configured QA helpers must report.
func TestAdvanceBlocksCleanQAWhenParallelMembersIncomplete(t *testing.T) {
	// Given: a clean QA artifact with only one successful parallel QA result.
	repo := gitRepo(t)
	runID := "parallel-qa-incomplete-run"
	base := runDir(repo, runID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWritePrompt(t, filepath.Join(base, "acceptance.json"), acceptanceJSON())
	mustWritePrompt(t, filepath.Join(base, "qa-1.json"), cleanQAJSON())
	mustWritePrompt(t, filepath.Join(base, "parallel-qa-1.json"), `{
		"group":"qa",
		"mode":"gate_input",
		"summary":"only CLI/API tester reported",
		"members":[{
			"name":"CLI/API 测试员",
			"purpose":"exercise command path",
			"status":"success",
			"summary":"CLI path passed"
		}]
	}`)
	state := State{RunID: runID, ChangeName: "demo", Stage: "qa_1", Status: statusRunning, Workflow: parallelWorkflowForTest()}

	// When: the state machine advances the QA stage.
	err := (&Engine{Repo: repo}).advance(&state)

	// Then: archive remains blocked because every missing configured QA member is reported.
	if err == nil ||
		!strings.Contains(err.Error(), "浏览器路径测试员") ||
		!strings.Contains(err.Error(), "证据采集员") ||
		!strings.Contains(err.Error(), "回归场景测试员") {
		t.Fatalf("advance should reject incomplete parallel QA coverage, got %v", err)
	}
}

// TestAdvanceAllowsCleanQAWhenParallelMembersComplete verifies complete successful QA input may pass.
func TestAdvanceAllowsCleanQAWhenParallelMembersComplete(t *testing.T) {
	// Given: clean QA and all configured parallel QA helpers reported success.
	repo := gitRepo(t)
	runID := "parallel-qa-complete-run"
	base := runDir(repo, runID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWritePrompt(t, filepath.Join(base, "acceptance.json"), acceptanceJSON())
	mustWritePrompt(t, filepath.Join(base, "qa-1.json"), cleanQAJSON())
	mustWritePrompt(t, filepath.Join(base, "parallel-qa-1.json"), parallelQAArtifactForTest())
	state := State{RunID: runID, ChangeName: "demo", Stage: "qa_1", Status: statusRunning, Workflow: parallelWorkflowForTest()}

	// When: the state machine advances the QA stage.
	err := (&Engine{Repo: repo}).advance(&state)

	// Then: archive is the next gate.
	if err != nil {
		t.Fatalf("advance should allow complete clean parallel QA, got %v", err)
	}
	if state.Stage != "archive" {
		t.Fatalf("stage = %s, want archive", state.Stage)
	}
}

func parallelWorkflowForTest() WorkflowConfig {
	workflow := DefaultWorkflowConfig()
	workflow.Parallel.Enabled = true
	return workflow
}

func writeDoneTask(t *testing.T, repo string) {
	t.Helper()
	mustChange(t, repo, "demo")
	mustWritePrompt(t, filepath.Join(repo, "docs", "changes", "demo", "task.md"), "- [x] task\n")
}
