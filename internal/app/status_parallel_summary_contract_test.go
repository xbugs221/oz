// Package app contains long-lived regression tests migrated from shell-injected contracts.
package app

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestHumanStatusHidesParallelFanInSummary 验证单 workflow status 只展示极简阶段和 helper 行，不展示 fan-in 摘要。
func TestHumanStatusHidesParallelFanInSummary(t *testing.T) {
	repo := gitRepo(t)
	workflow := statusParallelWorkflowForTest()
	runPath := runDir(repo, "parallel-status-single")
	targetArtifact := filepath.Join(runPath, "parallel-members", "review", "1", "target.json")
	riskArtifact := filepath.Join(runPath, "parallel-members", "review", "1", "risk.json")
	state := State{
		RunID:      "parallel-status-single",
		ChangeName: "demo",
		Sealed:     true,
		Status:     statusRunning,
		Stage:      "review_1",
		Stages:     map[string]string{"execution": "completed"},
		Sessions: map[string]string{
			"codex:executor": "writer-session",
			"codex:reviewer": "reviewer-session",
			"pi:subagent:review:目标核对审核员:1": "target-session",
			"pi:subagent:review:安全风险审核员:1": "risk-session",
		},
		DAGNodes: map[string]DAGNodeState{
			"before_review_1_1": {Status: "success", Artifact: targetArtifact},
			"before_review_1_2": {Status: "failed", Artifact: riskArtifact},
		},
		Workflow: workflow,
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(targetArtifact, ParallelMemberResult{Name: "目标核对审核员", Purpose: "核对目标", Status: "success", Summary: "target matches"}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(riskArtifact, ParallelMemberResult{Name: "安全风险审核员", Purpose: "检查风险", Status: "failed", Summary: "found a risk"}); err != nil {
		t.Fatal(err)
	}
	statusWriteParallelArtifact(t, filepath.Join(runPath, "parallel-implementation-context.json"), ParallelArtifact{
		Group:   "implementation_context",
		Mode:    "advisory",
		Summary: "implementation context completed",
		Members: []ParallelMemberResult{
			{Name: "代码库侦察员", Purpose: "梳理既有代码模式", Status: "success", Summary: "found local patterns"},
			{Name: "外部资料研究员", Purpose: "核对外部资料", Status: "success", Summary: "checked docs"},
		},
	})
	statusWriteParallelArtifact(t, filepath.Join(runPath, "parallel-review-1.json"), ParallelArtifact{
		Group:   "review",
		Mode:    "gate_input",
		Summary: "review helpers completed",
		Members: []ParallelMemberResult{
			{Name: "目标核对审核员", Purpose: "核对目标", Status: "success", Summary: "target matches"},
			{Name: "安全风险审核员", Purpose: "检查风险", Status: "failed", Summary: "found a risk"},
		},
	})

	var stdout bytes.Buffer
	if err := printHumanStatus(&stdout, repo, "-w1"); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	statusSaveResult(t, "status-single.txt", got)
	for _, want := range []string{
		"执行   writer-session   ✓",
		"审核   reviewer-session →",
		"目标 target-session   ✓",
		"风险 risk-session     x",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("status output missing %q:\n%s", want, got)
		}
	}
	for _, banned := range []string{"- 并行", "implementation_context", "parallel-review", "代码库侦察员 success", "安全风险审核员 failed", "Sisyphus", "Prometheus", "Metis", "Momus", "Oracle", "Explore"} {
		if strings.Contains(got, banned) {
			t.Fatalf("status output leaked internal agent name %q:\n%s", banned, got)
		}
	}
}

// TestParallelReviewGateAllowsMissingOrInvalidArtifact 验证 helper artifact 交付问题不覆盖主审核决策。
func TestParallelReviewGateAllowsMissingOrInvalidArtifact(t *testing.T) {
	repo := gitRepo(t)
	workflow := statusParallelWorkflowForTest()
	runPath := runDir(repo, "parallel-status-missing")
	if err := ValidateParallelReviewGate(runPath, workflow, 1, cleanReviewForParallelStatusTest()); err != nil {
		t.Fatalf("missing helper artifact must not block clean review: %v", err)
	}

	artifactPath := filepath.Join(runPath, "parallel-review-1.json")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifactPath, []byte("{not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateParallelReviewGate(runPath, workflow, 1, cleanReviewForParallelStatusTest()); err != nil {
		t.Fatalf("invalid helper artifact must not block clean review: %v", err)
	}
}

// TestParallelReviewGateAllowsUnconfiguredParallelMembers 验证未配置 helper 噪声不覆盖主审核决策。
func TestParallelReviewGateAllowsUnconfiguredParallelMembers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status string
	}{
		{name: "extra-success", status: "success"},
		{name: "extra-failed", status: "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := gitRepo(t)
			workflow := statusParallelWorkflowForTest()
			runPath := runDir(repo, "parallel-status-unconfigured-"+tc.name)
			statusWriteParallelArtifact(t, filepath.Join(runPath, "parallel-review-1.json"), ParallelArtifact{
				Group:   "review",
				Mode:    "gate_input",
				Summary: "review helpers completed",
				Members: []ParallelMemberResult{
					{Name: "目标核对审核员", Purpose: "核对目标", Status: "success", Summary: "target matches"},
					{Name: "安全风险审核员", Purpose: "检查风险", Status: "success", Summary: "no risk found"},
					{Name: "未配置审核员", Purpose: "should not be visible", Status: tc.status, Summary: "poisoned member"},
				},
			})
			if err := ValidateParallelReviewGate(runPath, workflow, 1, cleanReviewForParallelStatusTest()); err != nil {
				t.Fatalf("unconfigured helper member must not block clean review: %v", err)
			}
		})
	}
}

// TestBatchHumanStatusHidesParallelSummaryUnderChange 验证 batch status 保持 change 层级且不展示 fan-in 摘要。
func TestBatchHumanStatusHidesParallelSummaryUnderChange(t *testing.T) {
	repo := gitRepo(t)
	workflow := statusParallelWorkflowForTest()
	state := State{
		RunID:      "parallel-status-batch-run",
		ChangeName: "1-demo",
		Sealed:     true,
		Status:     statusRunning,
		Stage:      "execution",
		BatchID:    "parallel-status-batch",
		Sessions:   map[string]string{"codex:executor": "writer-session"},
		Workflow:   workflow,
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	statusWriteParallelArtifact(t, filepath.Join(runDir(repo, state.RunID), "parallel-implementation-context.json"), ParallelArtifact{
		Group:   "implementation_context",
		Mode:    "advisory",
		Summary: "implementation context completed",
		Members: []ParallelMemberResult{
			{Name: "代码库侦察员", Purpose: "梳理既有代码模式", Status: "success", Summary: "found local patterns"},
			{Name: "外部资料研究员", Purpose: "核对外部资料", Status: "success", Summary: "checked docs"},
		},
	})
	batch := BatchState{
		BatchID:      "parallel-status-batch",
		Status:       batchStatusRunning,
		Changes:      []string{"1-demo"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{"1-demo": state.RunID},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := printHumanStatus(&stdout, repo); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	statusSaveResult(t, "status-batch.txt", got)
	for _, want := range []string{
		"批量任务 b1 running 1/1",
		"- 1-demo",
		"执行 writer-session →",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("batch status output missing %q:\n%s", want, got)
		}
	}
	for _, banned := range []string{"- 并行", "implementation_context", "代码库侦察员 success", "外部资料研究员 success"} {
		if strings.Contains(got, banned) {
			t.Fatalf("batch status output leaked fan-in %q:\n%s", banned, got)
		}
	}
}

func statusParallelWorkflowForTest() WorkflowConfig {
	workflow := DefaultWorkflowConfig()
	workflow.Parallel.Enabled = true
	workflow.Parallel.Groups["implementation_context"] = ParallelGroupConfig{
		Mode: "advisory",
		Members: []ParallelMemberConfig{
			{Name: "代码库侦察员", Purpose: "梳理既有代码模式"},
			{Name: "外部资料研究员", Purpose: "核对外部资料"},
		},
	}
	workflow.Parallel.Groups["review"] = ParallelGroupConfig{
		Mode: "gate_input",
		Members: []ParallelMemberConfig{
			{Name: "目标核对审核员", Purpose: "核对目标"},
			{Name: "安全风险审核员", Purpose: "检查风险"},
		},
	}
	return workflow
}

func statusWriteParallelArtifact(t *testing.T, path string, artifact ParallelArtifact) {
	t.Helper()
	if err := writeJSONFile(path, artifact); err != nil {
		t.Fatal(err)
	}
}

// cleanReviewForParallelStatusTest 构造 clean review，专门触发 parallel gate 对 fan-in artifact 的校验。
func cleanReviewForParallelStatusTest() Review {
	return Review{
		Summary:  "review clean",
		Decision: "clean",
		Checks: ReviewChecks{
			OzAligned:                true,
			TasksVerified:            true,
			TestsMeaningful:          true,
			ImplementationScoped:     true,
			RuntimeBehaviorVerified:  true,
			PreviousFindingsResolved: true,
		},
		Evidence: []string{
			"validation artifact passed: validation-review-1.json",
			"runtime evidence: QA trace test-results/status-parallel.json",
		},
	}
}

func statusSaveResult(t *testing.T, name string, text string) {
	t.Helper()
	resultDir := os.Getenv("WO_STATUS_PARALLEL_RESULT_DIR")
	if resultDir == "" {
		return
	}
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resultDir, name), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test User"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-q", "-m", "initial"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	return repo
}
