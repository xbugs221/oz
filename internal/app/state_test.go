// Package app tests sealed run state machine progression and resume behavior.
package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeRunner struct{}

// Run writes the artifact expected by the current prompt and returns a stable thread id.
func (fakeRunner) Run(_ context.Context, repo, prompt, threadID string, options StageOptions) (string, error) {
	if output := subagentOutputFromPrompt(prompt); output != "" {
		name := promptValue(prompt, "SUBAGENT_NAME")
		purpose := promptValue(prompt, "SUBAGENT_PURPOSE")
		body := `{"name":"` + name + `","purpose":"` + purpose + `","status":"success","summary":"fake subagent completed","evidence":["fake-runner"]}` + "\n"
		if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
			return "", err
		}
		return "subagent-thread", os.WriteFile(output, []byte(body), 0o644)
	}
	stage := stageFromPromptOrState(repo, prompt)
	runID := currentRunID(repo)
	switch {
	case stage == "execution":
		if options.Reasoning != "low" || options.Fast {
			return "", os.ErrInvalid
		}
		return "executor-thread", os.WriteFile(filepath.Join(repo, "docs", "changes", "demo", "task.md"), []byte("- [x] task\n"), 0o644)
	case strings.HasPrefix(stage, "review_"):
		if options.Reasoning != "high" || options.Fast {
			return "", os.ErrInvalid
		}
		n := strings.TrimPrefix(stage, "review_")
		body := cleanReviewJSON()
		return "reviewer-thread", os.WriteFile(filepath.Join(runDir(repo, runID), "review-"+n+".json"), []byte(body), 0o644)
	case strings.HasPrefix(stage, "qa_"):
		if options.Reasoning != "high" || options.Fast {
			return "", os.ErrInvalid
		}
		n := strings.TrimPrefix(stage, "qa_")
		return "qa-thread", os.WriteFile(filepath.Join(runDir(repo, runID), "qa-"+n+".json"), []byte(cleanQAJSON()), 0o644)
	case stage == "archive":
		if err := os.MkdirAll(filepath.Join(repo, "docs", "changes", "archive", "2026-05-05-demo"), 0o755); err != nil {
			return "", err
		}
		return "archiver-thread", os.WriteFile(filepath.Join(runDir(repo, runID), "delivery-summary.md"), []byte("done\n"), 0o644)
	default:
		return threadID, nil
	}
}

func subagentOutputFromPrompt(prompt string) string {
	return promptValue(prompt, "SUBAGENT_OUTPUT")
}

func promptValue(prompt string, key string) string {
	prefix := key + "="
	for _, line := range strings.Split(prompt, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

type fakeTool struct {
	name   string
	runner AgentRunner
}

type subagentAwareRunner struct {
	runner AgentRunner
}

func (r subagentAwareRunner) SetProgress(writer io.Writer) {
	if runner, ok := r.runner.(progressSetter); ok {
		runner.SetProgress(writer)
	}
}

// Run creates read-only subagent artifacts for tests, then delegates main stages.
func (r subagentAwareRunner) Run(ctx context.Context, repo, prompt, threadID string, options StageOptions) (string, error) {
	if output := subagentOutputFromPrompt(prompt); output != "" {
		name := promptValue(prompt, "SUBAGENT_NAME")
		purpose := promptValue(prompt, "SUBAGENT_PURPOSE")
		body := `{"name":"` + name + `","purpose":"` + purpose + `","status":"success","summary":"fake subagent completed","evidence":["fake-runner"]}` + "\n"
		if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
			return "", err
		}
		return "subagent-thread", os.WriteFile(output, []byte(body), 0o644)
	}
	return r.runner.Run(ctx, repo, prompt, threadID, options)
}

// Name returns the configured backend name used by test workflow snapshots.
func (t fakeTool) Name() string { return t.name }

// Resolve keeps tests independent from real agent CLI binaries.
func (t fakeTool) Resolve() error { return nil }

// PlanningCommand is unused by sealed-run state machine tests.
func (t fakeTool) PlanningCommand(context.Context, string, string, io.Reader, StageOptions) (*exec.Cmd, error) {
	return nil, os.ErrInvalid
}

// NewRunner returns the fake sealed-run runner.
func (t fakeTool) NewRunner() AgentRunner { return t.runner }

// testRegistry routes codex stages to a deterministic fake runner.
func testRegistry(runner AgentRunner) *AgentRegistry {
	registry := &AgentRegistry{}
	wrapped := subagentAwareRunner{runner: runner}
	registry.Register(fakeTool{name: "codex", runner: wrapped})
	registry.Register(fakeTool{name: "opencode", runner: wrapped})
	registry.Register(fakeTool{name: "pi", runner: wrapped})
	return registry
}

type scenarioRunner struct {
	decisions   []string
	qaDecisions []string
	stages      []string
	prompts     []string
	tools       []string
	reasoning   []string
	fast        []bool
}

// Run records dynamic stages and writes artifacts matching the requested decision sequence.
func (r *scenarioRunner) Run(_ context.Context, repo, prompt, threadID string, options StageOptions) (string, error) {
	stage := stageFromPromptOrState(repo, prompt)
	r.stages = append(r.stages, stage)
	r.prompts = append(r.prompts, prompt)
	r.tools = append(r.tools, options.Tool)
	r.reasoning = append(r.reasoning, options.Reasoning)
	r.fast = append(r.fast, options.Fast)
	runID := currentRunID(repo)
	switch {
	case stage == "execution":
		return "executor-thread", os.WriteFile(filepath.Join(repo, "docs", "changes", "demo", "task.md"), []byte("- [x] task\n"), 0o644)
	case strings.HasPrefix(stage, "review_"):
		n := stageIteration(stage)
		decision := "clean"
		if n-1 < len(r.decisions) {
			decision = r.decisions[n-1]
		}
		body := cleanReviewJSON()
		if decision == "needs_fix" {
			body = fixReviewJSON()
		}
		return "reviewer-thread", os.WriteFile(filepath.Join(runDir(repo, runID), "review-"+strconv.Itoa(n)+".json"), []byte(body), 0o644)
	case strings.HasPrefix(stage, "qa_"):
		n := stageIteration(stage)
		body := cleanQAJSON()
		if n-1 < len(r.qaDecisions) && r.qaDecisions[n-1] == "needs_fix" {
			body = fixQAJSON()
		}
		return "qa-thread", os.WriteFile(filepath.Join(runDir(repo, runID), "qa-"+strconv.Itoa(n)+".json"), []byte(body), 0o644)
	case strings.HasPrefix(stage, "fix_"):
		n := strconv.Itoa(stageIteration(stage))
		return threadID, os.WriteFile(filepath.Join(runDir(repo, runID), "fix-"+n+"-summary.md"), []byte("done\n"), 0o644)
	case stage == "archive":
		if err := os.MkdirAll(filepath.Join(repo, "docs", "changes", "archive", "2026-05-05-demo"), 0o755); err != nil {
			return "", err
		}
		return "archiver-thread", os.WriteFile(filepath.Join(runDir(repo, runID), "delivery-summary.md"), []byte("done\n"), 0o644)
	default:
		return "", os.ErrInvalid
	}
}

type qaArtifactRepairRunner struct {
	scenarioRunner
	qaAttempts int
}

// Run writes an invalid clean QA artifact once, then repairs it on the same QA stage.
func (r *qaArtifactRepairRunner) Run(ctx context.Context, repo, prompt, threadID string, options StageOptions) (string, error) {
	stage := stageFromPromptOrState(repo, prompt)
	if !strings.HasPrefix(stage, "qa_") {
		return r.scenarioRunner.Run(ctx, repo, prompt, threadID, options)
	}
	r.stages = append(r.stages, stage)
	r.prompts = append(r.prompts, prompt)
	r.tools = append(r.tools, options.Tool)
	r.reasoning = append(r.reasoning, options.Reasoning)
	r.fast = append(r.fast, options.Fast)
	r.qaAttempts++
	runID := currentRunID(repo)
	n := strconv.Itoa(stageIteration(stage))
	body := cleanQAJSON()
	if r.qaAttempts == 1 {
		body = cleanQAWithUnknownAcceptanceIDJSON()
	}
	return "qa-thread", os.WriteFile(filepath.Join(runDir(repo, runID), "qa-"+n+".json"), []byte(body), 0o644)
}

type workflowFailureReviewRunner struct {
	scenarioRunner
}

// Run writes workflow_failure in review_2 after a no-progress fix summary.
func (r *workflowFailureReviewRunner) Run(ctx context.Context, repo, prompt, threadID string, options StageOptions) (string, error) {
	stage := stageFromPromptOrState(repo, prompt)
	if stage == "review_2" {
		r.stages = append(r.stages, stage)
		r.prompts = append(r.prompts, prompt)
		r.tools = append(r.tools, options.Tool)
		r.reasoning = append(r.reasoning, options.Reasoning)
		r.fast = append(r.fast, options.Fast)
		runID := currentRunID(repo)
		body := workflowFailureReviewJSON()
		return "reviewer-thread", os.WriteFile(filepath.Join(runDir(repo, runID), "review-2.json"), []byte(body), 0o644)
	}
	if stage == "fix_1" {
		r.stages = append(r.stages, stage)
		r.prompts = append(r.prompts, prompt)
		r.tools = append(r.tools, options.Tool)
		r.reasoning = append(r.reasoning, options.Reasoning)
		r.fast = append(r.fast, options.Fast)
		runID := currentRunID(repo)
		body := "未能完成修复：缺少必要凭据，当前 findings 无法在本轮工作流内可靠解决。\n"
		return threadID, os.WriteFile(filepath.Join(runDir(repo, runID), "fix-1-summary.md"), []byte(body), 0o644)
	}
	return r.scenarioRunner.Run(ctx, repo, prompt, threadID, options)
}

type sessionCaptureRunner struct {
	seen []string
}

// Run records incoming session ids and writes artifacts for execution and fix stages.
func (r *sessionCaptureRunner) Run(_ context.Context, repo, prompt, sessionID string, options StageOptions) (string, error) {
	stage := stageFromPromptOrState(repo, prompt)
	r.seen = append(r.seen, options.Tool+":"+stage+":"+sessionID)
	runID := currentRunID(repo)
	switch {
	case stage == "execution":
		return options.Tool + "-executor-session", os.WriteFile(filepath.Join(repo, "docs", "changes", "demo", "task.md"), []byte("- [x] task\n"), 0o644)
	case strings.HasPrefix(stage, "fix_"):
		n := strconv.Itoa(stageIteration(stage))
		return options.Tool + "-executor-session", os.WriteFile(filepath.Join(runDir(repo, runID), "fix-"+n+"-summary.md"), []byte("done\n"), 0o644)
	case stage == "archive":
		if err := os.MkdirAll(filepath.Join(repo, "docs", "changes", "archive", "2026-05-05-demo"), 0o755); err != nil {
			return "", err
		}
		return options.Tool + "-archiver-session", os.WriteFile(filepath.Join(runDir(repo, runID), "delivery-summary.md"), []byte("done\n"), 0o644)
	default:
		return sessionID, nil
	}
}

type validationRunner struct {
	decisions []string
	stages    []string
	prompts   []string
	attempts  map[string]int
}

// Run records validation-gated reruns and writes the normal stage artifact.
func (r *validationRunner) Run(_ context.Context, repo, prompt, threadID string, _ StageOptions) (string, error) {
	if r.attempts == nil {
		r.attempts = map[string]int{}
	}
	stage := stageFromPromptOrState(repo, prompt)
	r.stages = append(r.stages, stage)
	r.prompts = append(r.prompts, prompt)
	r.attempts[stage]++
	runID := currentRunID(repo)
	switch {
	case stage == "execution":
		if err := os.WriteFile(filepath.Join(repo, "docs", "changes", "demo", "task.md"), []byte("- [x] task\n"), 0o644); err != nil {
			return "", err
		}
		if r.attempts[stage] > 1 {
			return "executor-thread", os.WriteFile(filepath.Join(repo, "validation-ok"), []byte("ok\n"), 0o644)
		}
		return "executor-thread", nil
	case strings.HasPrefix(stage, "review_"):
		n := stageIteration(stage)
		decision := "clean"
		if n-1 < len(r.decisions) {
			decision = r.decisions[n-1]
		}
		body := cleanReviewJSON()
		if decision == "needs_fix" {
			body = fixReviewJSON()
		}
		return "reviewer-thread", os.WriteFile(filepath.Join(runDir(repo, runID), "review-"+strconv.Itoa(n)+".json"), []byte(body), 0o644)
	case strings.HasPrefix(stage, "qa_"):
		n := stageIteration(stage)
		return "qa-thread", os.WriteFile(filepath.Join(runDir(repo, runID), "qa-"+strconv.Itoa(n)+".json"), []byte(cleanQAJSON()), 0o644)
	case strings.HasPrefix(stage, "fix_"):
		n := strconv.Itoa(stageIteration(stage))
		if err := os.WriteFile(filepath.Join(runDir(repo, runID), "fix-"+n+"-summary.md"), []byte("done\n"), 0o644); err != nil {
			return "", err
		}
		if r.attempts[stage] > 1 {
			return "executor-thread", os.WriteFile(filepath.Join(repo, "validation-ok"), []byte("ok\n"), 0o644)
		}
		_ = os.Remove(filepath.Join(repo, "validation-ok"))
		return "executor-thread", nil
	case stage == "archive":
		if err := os.MkdirAll(filepath.Join(repo, "docs", "changes", "archive", "2026-05-05-demo"), 0o755); err != nil {
			return "", err
		}
		return "archiver-thread", os.WriteFile(filepath.Join(runDir(repo, runID), "delivery-summary.md"), []byte("done\n"), 0o644)
	default:
		return threadID, nil
	}
}

type slowSubagentRunner struct {
	mu      sync.Mutex
	windows map[string]timeWindow
}

type timeWindow struct {
	start  time.Time
	finish time.Time
}

// Run records subagent execution windows and writes normal workflow artifacts.
func (r *slowSubagentRunner) Run(_ context.Context, repo, prompt, threadID string, options StageOptions) (string, error) {
	if output := subagentOutputFromPrompt(prompt); output != "" {
		name := promptValue(prompt, "SUBAGENT_NAME")
		purpose := promptValue(prompt, "SUBAGENT_PURPOSE")
		start := time.Now()
		time.Sleep(120 * time.Millisecond)
		body := `{"name":"` + name + `","purpose":"` + purpose + `","status":"success","summary":"slow fake subagent completed","evidence":["fake-runner"]}` + "\n"
		if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(output, []byte(body), 0o644); err != nil {
			return "", err
		}
		r.mu.Lock()
		if r.windows == nil {
			r.windows = map[string]timeWindow{}
		}
		r.windows[name] = timeWindow{start: start, finish: time.Now()}
		r.mu.Unlock()
		return "subagent-thread", nil
	}
	return fakeRunner{}.Run(context.Background(), repo, prompt, threadID, options)
}

func stageFromPromptOrState(repo, prompt string) string {
	stage := strings.TrimSpace(strings.Split(prompt, "\n")[0])
	if _, err := roleForStage(stage); err == nil {
		return stage
	}
	runID := currentRunID(repo)
	if runID == "" {
		return strings.TrimSpace(prompt)
	}
	state, err := loadState(repo, runID)
	if err != nil || state.Stage == "" {
		return strings.TrimSpace(prompt)
	}
	return state.Stage
}

// TestEngineStartRunsCleanReviewsToDone verifies the main clean path through three reviews and archive.
func TestEngineStartRunsCleanReviewsToDone(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	engine := NewEngine(repo, testRegistry(fakeRunner{}))
	var progress bytes.Buffer
	engine.Output = &progress
	if err := engine.Start(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	runID, err := newestRun(repo)
	if err != nil {
		t.Fatal(err)
	}
	state, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != statusDone || state.Stage != "done" {
		t.Fatalf("state = %s/%s, want done/done", state.Status, state.Stage)
	}
	if state.Sessions["codex:executor"] == "" || state.Sessions["codex:reviewer"] == "" || state.Sessions["codex:archiver"] == "" {
		t.Fatalf("sessions not saved: %#v", state.Sessions)
	}
	for _, stage := range []string{"execution", "review_1", "archive"} {
		timing := state.StageTimings[stage]
		if timing.StartedAt == "" || timing.FinishedAt == "" {
			t.Fatalf("%s timing not saved: %#v", stage, state.StageTimings)
		}
		if _, err := time.Parse(time.RFC3339Nano, timing.StartedAt); err != nil {
			t.Fatalf("%s started_at is not RFC3339Nano: %v", stage, err)
		}
		if _, err := time.Parse(time.RFC3339Nano, timing.FinishedAt); err != nil {
			t.Fatalf("%s finished_at is not RFC3339Nano: %v", stage, err)
		}
	}
	if len(state.Paths) != 0 {
		t.Fatalf("paths = %#v, want no duplicated runtime logs", state.Paths)
	}
	if _, err := os.Stat(filepath.Join(runDir(repo, runID), "logs")); !os.IsNotExist(err) {
		t.Fatalf("logs directory exists or cannot be checked: %v", err)
	}
	if !strings.Contains(progress.String(), "- 写 未知 →") {
		t.Fatalf("progress missing execution start: %q", progress.String())
	}
	if !strings.Contains(progress.String(), "- 存 archiver-thread ✓") {
		t.Fatalf("progress missing done line: %q", progress.String())
	}
	if strings.Contains(progress.String(), "✓→") {
		t.Fatalf("completed stages should not remain marked running: %q", progress.String())
	}
}

// TestCreateRunSnapshotsAcceptanceContract verifies sealed runs keep their own contract copy.
func TestCreateRunSnapshotsAcceptanceContract(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	engine := NewEngine(repo, testRegistry(fakeRunner{}))
	state, err := engine.createRun("demo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAcceptance(filepath.Join(runDir(repo, state.RunID), "acceptance.json")); err != nil {
		t.Fatalf("run acceptance snapshot missing or invalid: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(repo, "docs", "changes", "demo")); err != nil {
		t.Fatal(err)
	}
	if _, err := readAcceptanceForState(repo, state); err != nil {
		t.Fatalf("sealed run should read acceptance snapshot after active change is gone: %v", err)
	}
}

// TestArchiveReadinessReadsArchivedAcceptance verifies oz archive does not break final gates.
func TestArchiveReadinessReadsArchivedAcceptance(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	runID := "archive-after-oz-run"
	base := runDir(repo, runID)
	archive := filepath.Join(repo, "docs", "changes", "archive", "2026-06-04-demo")
	if err := os.MkdirAll(archive, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWritePrompt(t, filepath.Join(archive, "acceptance.json"), acceptanceJSON())
	if err := os.RemoveAll(filepath.Join(repo, "docs", "changes", "demo")); err != nil {
		t.Fatal(err)
	}
	mustWritePrompt(t, filepath.Join(base, "delivery-summary.md"), "done\n")
	mustWritePrompt(t, filepath.Join(base, "review-1.json"), cleanReviewJSON())
	mustWritePrompt(t, filepath.Join(base, "qa-1.json"), cleanQAJSON())
	state := State{
		RunID:      runID,
		ChangeName: "demo",
		Status:     statusRunning,
		Stage:      "archive",
		Stages: map[string]string{
			"execution": "completed",
			"review_1":  "completed",
			"qa_1":      "completed",
		},
		Workflow: DefaultWorkflowConfig(),
	}
	if err := NewEngine(repo, testRegistry(fakeRunner{})).validateArchiveReadiness(state); err != nil {
		t.Fatalf("archive readiness should read archived acceptance after active change is gone: %v", err)
	}
}

// TestEngineSupportsZeroReviewIterations verifies execution can go straight to archive.
func TestEngineSupportsZeroReviewIterations(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  workflow:\n    max_review_iterations: 0\n")
	runner := &scenarioRunner{}
	engine := NewEngine(repo, testRegistry(runner))
	if err := engine.Start(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(runner.stages, ","); got != "execution,archive" {
		t.Fatalf("stages = %s, want execution/archive", got)
	}
}

// TestGoDAGRunsReadySubagentsConcurrently verifies fan-out members overlap before fan-in.
func TestGoDAGRunsReadySubagentsConcurrently(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  workflow:\n    max_review_iterations: 0\n")
	runner := &slowSubagentRunner{}
	registry := &AgentRegistry{}
	registry.Register(fakeTool{name: "codex", runner: runner})
	registry.Register(fakeTool{name: "opencode", runner: runner})
	registry.Register(fakeTool{name: "pi", runner: runner})
	engine := NewEngine(repo, registry)
	if err := engine.Start(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.windows) < 2 {
		t.Fatalf("subagent windows missing: %#v", runner.windows)
	}
	overlapped := false
	for firstName, first := range runner.windows {
		for secondName, second := range runner.windows {
			if firstName == secondName {
				continue
			}
			if first.start.Before(second.finish) && second.start.Before(first.finish) {
				overlapped = true
			}
		}
	}
	if !overlapped {
		t.Fatalf("subagents did not overlap: %#v", runner.windows)
	}
}

// TestValidationGateRerunsExecutionBeforeReview verifies failed commands return to executor first.
func TestValidationGateRerunsExecutionBeforeReview(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  workflow:\n    max_review_iterations: 0\n    parallel:\n      enabled: false\n    validation:\n      max_attempts_per_stage: 2\n      commands:\n        - test -f validation-ok\n")
	runner := &validationRunner{}
	engine := NewEngine(repo, testRegistry(runner))
	if err := engine.Start(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(runner.stages, ","); got != "execution,execution,archive" {
		t.Fatalf("stages = %s, want validation rerun before archive", got)
	}
	if !strings.Contains(runner.prompts[1], "Validation gate failed") {
		t.Fatalf("second execution prompt did not include validation failure:\n%s", runner.prompts[1])
	}
	runID, err := newestRun(repo)
	if err != nil {
		t.Fatal(err)
	}
	state, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Validation["execution"].Attempts != 2 || state.Validation["execution"].Status != validationStatusPassed {
		t.Fatalf("validation state = %#v, want two attempts and passed", state.Validation["execution"])
	}
}

// TestValidationGateBlocksAtAttemptLimit verifies failing commands stop before review/archive.
func TestValidationGateBlocksAtAttemptLimit(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  workflow:\n    max_review_iterations: 0\n    parallel:\n      enabled: false\n    validation:\n      max_attempts_per_stage: 1\n      commands:\n        - test -f never-created\n")
	runner := &scenarioRunner{}
	engine := NewEngine(repo, testRegistry(runner))
	if err := engine.Start(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	runID, err := newestRun(repo)
	if err != nil {
		t.Fatal(err)
	}
	state, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != statusValidationBlocked || state.Stage != statusValidationBlocked {
		t.Fatalf("state = %s/%s, want validation blocked", state.Status, state.Stage)
	}
	if got := strings.Join(runner.stages, ","); got != "execution" {
		t.Fatalf("stages = %s, want no archive after validation failure", got)
	}
}

// TestValidationGateRerunsFixWithoutConsumingReview verifies fix retries stay in the same round.
func TestValidationGateRerunsFixWithoutConsumingReview(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	if err := os.WriteFile(filepath.Join(repo, "validation-ok"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  workflow:\n    max_review_iterations: 2\n    parallel:\n      enabled: false\n    validation:\n      max_attempts_per_stage: 2\n      commands:\n        - test -f validation-ok\n")
	runner := &validationRunner{decisions: []string{"needs_fix", "clean"}}
	engine := NewEngine(repo, testRegistry(runner))
	if err := engine.Start(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(runner.stages, ","); got != "execution,review_1,fix_1,fix_1,review_2,qa_2,archive" {
		t.Fatalf("stages = %s, want fix validation to pass without extra review", got)
	}
}

// TestQAFailureReturnsToFix verifies runtime QA findings enter the normal fix/review loop.
func TestQAFailureReturnsToFix(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  workflow:\n    max_review_iterations: 2\n")
	runner := &scenarioRunner{decisions: []string{"clean", "clean"}, qaDecisions: []string{"needs_fix", "clean"}}
	engine := NewEngine(repo, testRegistry(runner))
	if err := engine.Start(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(runner.stages, ","); got != "execution,review_1,qa_1,fix_1,review_2,qa_2,archive" {
		t.Fatalf("stages = %s, want QA failure to re-enter fix/review before archive", got)
	}
}

// TestQAArtifactGateRerunsQAInsteadOfFailing verifies malformed QA coverage is sent back to QA.
func TestQAArtifactGateRerunsQAInsteadOfFailing(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  workflow:\n    engine: go-dag\n    max_review_iterations: 1\n    parallel:\n      enabled: false\n")
	runner := &qaArtifactRepairRunner{}
	engine := NewEngine(repo, testRegistry(runner))
	if err := engine.Start(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(runner.stages, ","); got != "execution,review_1,qa_1,qa_1,archive" {
		t.Fatalf("stages = %s, want QA artifact gate to rerun qa_1 before archive", got)
	}
	if len(runner.prompts) < 4 || !strings.Contains(runner.prompts[3], "Stage artifact gate failed") ||
		!strings.Contains(runner.prompts[3], "not-in-acceptance") {
		t.Fatalf("second QA prompt did not include artifact gate feedback:\n%s", strings.Join(runner.prompts, "\n---\n"))
	}
	runID, err := newestRun(repo)
	if err != nil {
		t.Fatal(err)
	}
	state, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != statusDone || state.Stage != "done" {
		t.Fatalf("state = %s/%s, want done/done", state.Status, state.Stage)
	}
	qaGate := state.Validation["qa_1"]
	if qaGate.Kind != validationKindArtifact || qaGate.Status != validationStatusPassed || qaGate.Attempts != 1 || qaGate.LastError != "" {
		t.Fatalf("qa artifact gate state = %#v, want one repaired artifact-gate failure", qaGate)
	}
}

// TestSubagentPromptsSpellStrictFindingSchema keeps real pi outputs inside the member artifact contract.
func TestSubagentPromptsSpellStrictFindingSchema(t *testing.T) {
	member := ParallelMemberConfig{Name: "需求分析员", Purpose: "找出需求歧义、风险和遗漏"}
	initial := subagentPrompt("planning_context", member, "/tmp/member.json")
	retry := artifactRetryPrompt("planning_context", member, "/tmp/member.json", fmt.Errorf(`json: unknown field "category"`))
	for label, prompt := range map[string]string{"initial": initial, "retry": retry} {
		for _, want := range []string{
			"只允许字段：name, purpose, status, summary, evidence, findings",
			"每个对象只允许 title, severity, evidence, recommendation",
			"不要使用 category、description、detail、location、level、type",
			"需要分类或位置时写入 title/evidence/recommendation",
		} {
			if !strings.Contains(prompt, want) {
				t.Fatalf("%s prompt missing %q:\n%s", label, want, prompt)
			}
		}
	}
	if !strings.Contains(retry, `json: unknown field "category"`) {
		t.Fatalf("retry prompt missing concrete schema error:\n%s", retry)
	}
}

// TestGoDAGAdvanceArtifactGateFailurePersistsRetry verifies advance-time archive gates survive node reload.
func TestGoDAGAdvanceArtifactGateFailurePersistsRetry(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	runID := "archive-readiness-run"
	if err := os.MkdirAll(runDir(repo, runID), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir(repo, runID), "delivery-summary.md"), []byte("done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "docs", "changes", "archive", "2026-05-05-demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	state := State{
		RunID:      runID,
		ChangeName: "demo",
		Sealed:     true,
		Status:     statusRunning,
		Stage:      "archive",
		Sessions:   map[string]string{},
		Stages:     map[string]string{"execution": "completed", "archive": "completed"},
		Workflow:   DefaultWorkflowConfig(),
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine(repo, testRegistry(fakeRunner{}))
	var out bytes.Buffer
	err := engine.nodeRunStage(context.Background(), state, []string{"--stage", "archive"}, &out)
	if err == nil || !strings.Contains(err.Error(), "archive 阶段缺少 clean QA artifact") {
		t.Fatalf("nodeRunStage err = %v, want archive readiness artifact gate", err)
	}
	persisted, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	gate := persisted.Validation["archive"]
	if gate.Kind != validationKindArtifact || gate.Status != validationStatusFailed || gate.Attempts != 1 {
		t.Fatalf("archive validation = %#v, want persisted artifact gate failure", gate)
	}
	if persisted.Stages["archive"] != "validation_failed" {
		t.Fatalf("archive stage = %q, want validation_failed", persisted.Stages["archive"])
	}
	node := WorkflowNode{ID: "archive", Type: "main_stage", Stage: "archive"}
	if !engine.goDAGShouldRetryNode(runID, node) {
		t.Fatal("go-dag should retry archive after persisted artifact gate failure")
	}
}

// TestLegacyRunLoopDoesNotPrecheckMissingNewStageArtifact verifies first turns are not false retries.
func TestLegacyRunLoopDoesNotPrecheckMissingNewStageArtifact(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	runID := "legacy-new-review-run"
	mustSnapshotPrompts(t, repo, runID)
	head, diff, err := gitSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	workflow := DefaultWorkflowConfig()
	workflow.Engine = "legacy"
	workflow.Parallel.Enabled = false
	state := State{
		RunID:        runID,
		ChangeName:   "demo",
		Sealed:       true,
		Status:       statusRunning,
		Stage:        "review_1",
		BaselineHead: head,
		BaselineDiff: diff,
		Sessions:     map[string]string{},
		Stages:       map[string]string{"execution": "completed"},
		Workflow:     workflow,
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	runner := &scenarioRunner{}
	engine := NewEngine(repo, testRegistry(runner))

	if err := engine.runLoop(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if len(runner.prompts) == 0 {
		t.Fatal("legacy runLoop did not run the review agent")
	}
	if strings.Contains(runner.prompts[0], "Stage artifact gate failed") {
		t.Fatalf("first review prompt had false artifact gate retry:\n%s", runner.prompts[0])
	}
	final, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Validation["review_1"].Attempts != 0 {
		t.Fatalf("review validation attempts = %d, want no pre-run artifact gate attempt", final.Validation["review_1"].Attempts)
	}
}

// TestWorkflowFailureReviewFailsWorkflow verifies review JSON can stop futile fix loops.
func TestWorkflowFailureReviewFailsWorkflow(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  workflow:\n    max_review_iterations: 3\n")
	runner := &workflowFailureReviewRunner{scenarioRunner: scenarioRunner{decisions: []string{"needs_fix", "clean"}}}
	engine := NewEngine(repo, testRegistry(runner))
	if err := engine.Start(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	runID, err := newestRun(repo)
	if err != nil {
		t.Fatal(err)
	}
	state, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != statusFailed || state.Stage != "review_2" {
		t.Fatalf("state = %s/%s, want failed/review_2", state.Status, state.Stage)
	}
	if !strings.Contains(state.Error, "上游接口缺少必要凭据") {
		t.Fatalf("error = %q, want workflow failure reason", state.Error)
	}
	if got := strings.Join(runner.stages, ","); got != "execution,review_1,fix_1,review_2" {
		t.Fatalf("stages = %s, want no fix after workflow failure review", got)
	}
}

// TestEngineBlocksAfterLastFix verifies the final fix does not auto-archive.
func TestEngineBlocksAfterLastFix(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  workflow:\n    max_review_iterations: 1\n")
	runner := &scenarioRunner{decisions: []string{"needs_fix"}}
	engine := NewEngine(repo, testRegistry(runner))
	if err := engine.Start(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	runID, err := newestRun(repo)
	if err != nil {
		t.Fatal(err)
	}
	state, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != statusBlocked || state.Stage != statusBlocked {
		t.Fatalf("state = %s/%s, want blocked", state.Status, state.Stage)
	}
	if strings.Contains(strings.Join(runner.stages, ","), "archive") {
		t.Fatalf("stages = %v, archive should not run", runner.stages)
	}
}

// TestRepeatedReviewFailuresEscalateFixEveryRound verifies repeated reviews climb one tier per failed round.
func TestRepeatedReviewFailuresEscalateFixGradually(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  workflow:\n    max_review_iterations: 4\n")
	runner := &scenarioRunner{decisions: []string{"needs_fix", "needs_fix", "needs_fix", "needs_fix"}}
	engine := NewEngine(repo, testRegistry(runner))
	if err := engine.Start(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(runner.stages, ","); got != "execution,review_1,fix_1,review_2,fix_2,review_3,fix_3,review_4,fix_4" {
		t.Fatalf("stages = %s, want four review/fix rounds", got)
	}
	fixOne := -1
	fixTwo := -1
	fixThree := -1
	fixFour := -1
	for i, stage := range runner.stages {
		if stage == "fix_1" {
			fixOne = i
		}
		if stage == "fix_2" {
			fixTwo = i
		}
		if stage == "fix_3" {
			fixThree = i
		}
		if stage == "fix_4" {
			fixFour = i
		}
	}
	if fixOne < 0 || fixTwo < 0 || fixThree < 0 || fixFour < 0 {
		t.Fatalf("fix indices = %d/%d/%d/%d, want all fix rounds", fixOne, fixTwo, fixThree, fixFour)
	}
	if runner.reasoning[fixOne] != "low" {
		t.Fatalf("fix_1 reasoning = %s, want default low", runner.reasoning[fixOne])
	}
	if runner.reasoning[fixTwo] != "medium" || runner.fast[fixTwo] {
		t.Fatalf("fix_2 options = reasoning:%s fast:%v, want medium/false", runner.reasoning[fixTwo], runner.fast[fixTwo])
	}
	if runner.reasoning[fixThree] != "high" || runner.fast[fixThree] {
		t.Fatalf("fix_3 options = reasoning:%s fast:%v, want high/false", runner.reasoning[fixThree], runner.fast[fixThree])
	}
	if runner.reasoning[fixFour] != "xhigh" || runner.fast[fixFour] {
		t.Fatalf("fix_4 options = reasoning:%s fast:%v, want xhigh/false", runner.reasoning[fixFour], runner.fast[fixFour])
	}
	runID, err := newestRun(repo)
	if err != nil {
		t.Fatal(err)
	}
	state, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	fixOption, err := state.Workflow.StageOption("fix_4")
	if err != nil {
		t.Fatal(err)
	}
	if fixOption.Reasoning != "xhigh" || fixOption.Fast {
		t.Fatalf("persisted fix_4 options = %#v, want xhigh/false", fixOption)
	}
}

// TestAcceptanceCodexDefaultPath verifies the default sealed workflow uses Codex for all stages.
func TestAcceptanceCodexDefaultPath(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	runner := &scenarioRunner{}
	engine := NewEngine(repo, testRegistry(runner))
	if err := engine.Start(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(runner.tools, ","); got != "codex,codex,codex,codex" {
		t.Fatalf("tools = %s, want codex default execution/review/archive path", got)
	}
}

// TestAcceptanceOpenCodeMinimalSealedRun verifies OpenCode can own a minimal sealed workflow.
func TestAcceptanceOpenCodeMinimalSealedRun(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), `
wo:
  workflow:
    max_review_iterations: 0
    defaults:
      tool: opencode
`)
	runner := &scenarioRunner{}
	engine := NewEngine(repo, testRegistry(runner))
	if err := engine.Start(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(runner.stages, ","); got != "execution,archive" {
		t.Fatalf("stages = %s, want minimal execution/archive", got)
	}
	if got := strings.Join(runner.tools, ","); got != "opencode,opencode" {
		t.Fatalf("tools = %s, want opencode minimal sealed run", got)
	}
}

// TestAcceptanceMixedToolWorkflow verifies execution/review/fix can use different tools.
func TestAcceptanceMixedToolWorkflow(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), `
wo:
  workflow:
    max_review_iterations: 1
    stages:
      execution:
        tool: opencode
      review:
        tool: codex
      fix:
        tool: opencode
      archive:
        tool: opencode
`)
	runner := &scenarioRunner{decisions: []string{"needs_fix"}}
	engine := NewEngine(repo, testRegistry(runner))
	if err := engine.Start(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(runner.stages, ","); got != "execution,review_1,fix_1" {
		t.Fatalf("stages = %s, want execution/review/fix before review limit", got)
	}
	if got := strings.Join(runner.tools, ","); got != "opencode,codex,opencode" {
		t.Fatalf("tools = %s, want mixed opencode/codex/opencode path", got)
	}
	state, err := loadState(repo, currentRunID(repo))
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != statusBlocked || state.Stage != statusBlocked || state.Error == "" {
		t.Fatalf("state = %#v, want blocked_review_limit with readable error", state)
	}
	if archiveExists(repo, "demo") {
		t.Fatal("blocked review limit must not enter archive")
	}
}

// TestWorkflowStagesExpandsConfiguredReviewIterations verifies dynamic stage expansion.
func TestWorkflowStagesExpandsConfiguredReviewIterations(t *testing.T) {
	for _, tc := range []struct {
		max  int
		want int
	}{
		{max: 0, want: 2},
		{max: 1, want: 5},
		{max: 3, want: 11},
		{max: 5, want: 17},
	} {
		config := DefaultWorkflowConfig()
		config.MaxReviewIterations = tc.max
		if got := len(workflowStagesForConfig(config)); got != tc.want {
			t.Fatalf("max %d stage count = %d, want %d", tc.max, got, tc.want)
		}
	}
}

// TestResumeUsesStateAndPromptSnapshots verifies current wo.yaml and prompts do not affect resume.
func TestResumeUsesStateAndPromptSnapshots(t *testing.T) {
	repo := gitRepo(t)
	runID := "snapshot-run"
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	mustSnapshotPrompts(t, repo, runID)
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  workflow:\n    max_review_iterations: 3\n")
	mustWritePrompt(t, filepath.Join(repo, ".wo", "cmd", "wo-start.md"), "changed-current-template")
	head, diff, err := gitSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	workflow := DefaultWorkflowConfig()
	workflow.Engine = "legacy"
	workflow.MaxReviewIterations = 0
	workflow.Parallel.Enabled = false
	workflow.Stages = map[string]StageOptions{
		"execution": {Reasoning: "low", Fast: false},
		"archive":   {Reasoning: "low", Fast: false},
	}
	state := State{
		RunID:        runID,
		ChangeName:   "demo",
		Sealed:       true,
		Status:       statusRunning,
		Stage:        "execution",
		BaselineHead: head,
		BaselineDiff: diff,
		Sessions:     map[string]string{},
		Stages:       map[string]string{},
		Workflow:     workflow,
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	runner := &scenarioRunner{}
	engine := NewEngine(repo, testRegistry(runner))
	if err := engine.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(runner.stages, ","); got != "execution,archive" {
		t.Fatalf("stages = %s, want state snapshot stages", got)
	}
	if got := runner.prompts[0]; !strings.Contains(got, "execution") || strings.Contains(got, "changed-current-template") {
		t.Fatalf("execution prompt = %q, want sealed snapshot without current template", got)
	}
}

// TestSessionKeysAreIsolatedByToolAndRole verifies mixed tools do not cross-resume sessions.
func TestSessionKeysAreIsolatedByToolAndRole(t *testing.T) {
	repo := gitRepo(t)
	runID := "session-run"
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	mustSnapshotPrompts(t, repo, runID)
	head, diff, err := gitSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	workflow := DefaultWorkflowConfig()
	workflow.MaxReviewIterations = 1
	workflow.Stages = map[string]StageOptions{
		"fix_1": {Tool: "codex", Reasoning: "low", Fast: false},
	}
	state := State{
		RunID:        runID,
		ChangeName:   "demo",
		Sealed:       true,
		Status:       statusRunning,
		Stage:        "fix_1",
		BaselineHead: head,
		BaselineDiff: diff,
		Sessions: map[string]string{
			"opencode:executor": "opencode-session",
		},
		Stages:   map[string]string{},
		Workflow: workflow,
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	runner := &sessionCaptureRunner{}
	engine := NewEngine(repo, testRegistry(runner))
	if err := engine.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(runner.seen, ","); got != "codex:fix_1:" {
		t.Fatalf("seen sessions = %q, want codex fix without opencode session", got)
	}
	final, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Sessions["codex:fixer"] == "" || final.Sessions["codex:executor"] != "" || final.Sessions["opencode:executor"] != "opencode-session" {
		t.Fatalf("sessions = %#v, want isolated fixer session", final.Sessions)
	}
}

// TestArchiveUsesIndependentArchiverSession verifies archive never resumes executor sessions.
func TestArchiveUsesIndependentArchiverSession(t *testing.T) {
	repo := gitRepo(t)
	runID := "archive-session-run"
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	mustSnapshotPrompts(t, repo, runID)
	head, diff, err := gitSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	state := State{
		RunID:        runID,
		ChangeName:   "demo",
		Sealed:       true,
		Status:       statusRunning,
		Stage:        "archive",
		BaselineHead: head,
		BaselineDiff: diff,
		Sessions:     map[string]string{"codex:executor": "executor-session"},
		Stages:       map[string]string{"execution": "completed"},
		Workflow:     zeroReviewWorkflow(),
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	runner := &sessionCaptureRunner{}
	engine := NewEngine(repo, testRegistry(runner))
	if err := engine.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(runner.seen, ","); got != "codex:archive:" {
		t.Fatalf("seen sessions = %q, want archive without executor resume", got)
	}
	final, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Sessions["codex:archiver"] == "" || final.Sessions["codex:executor"] != "executor-session" {
		t.Fatalf("sessions = %#v, want independent archiver session", final.Sessions)
	}
}

// TestPromptForStageRequiresSealedSnapshot verifies resume cannot read current templates.
func TestPromptForStageRequiresSealedSnapshot(t *testing.T) {
	repo := t.TempDir()
	mustWritePrompt(t, filepath.Join(repo, ".wo", "cmd", "wo-start.md"), "{{.Stage}}")
	_, err := promptForStage(repo, State{RunID: "sealed-run", Stage: "execution", Sealed: true})
	if err == nil {
		t.Fatal("expected missing run prompt snapshot to fail")
	}
}

// TestSnapshotRunPromptsWritesYAMLOnly verifies new sealed runs use one YAML snapshot.
func TestSnapshotRunPromptsWritesYAMLOnly(t *testing.T) {
	repo := gitRepo(t)
	runID := "yaml-snapshot-run"
	mustPrompts(t, repo)

	mustSnapshotPrompts(t, repo, runID)

	runPath := runDir(repo, runID)
	if _, err := os.Stat(filepath.Join(runPath, "prompt-snapshot.yaml")); err != nil {
		t.Fatalf("prompt-snapshot.yaml missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runPath, "prompts")); !os.IsNotExist(err) {
		t.Fatalf("legacy prompts dir err = %v, want not exist", err)
	}
	data, err := os.ReadFile(filepath.Join(runPath, "prompt-snapshot.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, key := range []string{"planning:", "execution:", "review:", "qa:", "fix:", "archive:"} {
		if !strings.Contains(body, key) {
			t.Fatalf("snapshot missing %s:\n%s", key, body)
		}
	}
	if strings.Contains(body, "writing:") {
		t.Fatalf("snapshot should not write new prompts.writing:\n%s", body)
	}
}

// TestPromptSnapshotPreservesCustomYAML verifies resume uses frozen YAML prompt bytes.
func TestPromptSnapshotPreservesCustomYAML(t *testing.T) {
	repo := gitRepo(t)
	runID := "custom-yaml-run"
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), `wo:
  prompts:
    planning: planning {{.Stage}}
    execution: |
      snapshot execution {{.Stage}}
      line two
    review: review {{.Stage}}
    fix: fix {{.Stage}}
    qa: qa {{.Stage}}
    archive: archive {{.Stage}}
`)
	mustSnapshotPrompts(t, repo, runID)
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), `wo:
  prompts:
    planning: planning
    execution: changed current {{.Stage}}
    review: changed review
    fix: changed fix
    qa: changed qa
    archive: changed archive
`)

	got, err := promptForStage(repo, State{RunID: runID, Stage: "execution", Sealed: true})
	if err != nil {
		t.Fatal(err)
	}
	want := "snapshot execution execution\nline two\n"
	if got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}
}

// TestPromptSnapshotUsesCurrentKeys verifies execution and fix snapshots are independent current keys.
func TestPromptSnapshotUsesCurrentKeys(t *testing.T) {
	repo := gitRepo(t)
	runID := "current-key-run"
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), `wo:
  prompts:
    planning: planning
    execution: "execution {{.Stage}}\n"
    review: "review {{.Stage}}\n"
    fix: "fix {{.Stage}}\n"
    qa: "qa {{.Stage}}\n"
    archive: archive
`)
	mustSnapshotPrompts(t, repo, runID)

	for _, tc := range []struct {
		stage string
		want  string
	}{
		{stage: "execution", want: "execution execution\n"},
		{stage: "fix_1", want: "fix fix_1\n"},
	} {
		got, err := promptForStage(repo, State{RunID: runID, Stage: tc.stage, Sealed: true})
		if err != nil {
			t.Fatalf("%s: %v", tc.stage, err)
		}
		if got != tc.want {
			t.Fatalf("%s prompt = %q, want %q", tc.stage, got, tc.want)
		}
	}
}

// TestPromptSnapshotRejectsWritingOnlySnapshot verifies old YAML snapshots fail closed.
func TestPromptSnapshotRejectsWritingOnlySnapshot(t *testing.T) {
	repo := gitRepo(t)
	runID := "writing-only-yaml-run"
	mustWritePrompt(t, filepath.Join(runDir(repo, runID), "prompt-snapshot.yaml"), "prompts:\n  writing: \"legacy {{.Stage}}\\n\"\n")

	for _, stage := range []string{"execution", "qa_1", "fix_1"} {
		if got, err := promptForStage(repo, State{RunID: runID, Stage: stage, Sealed: true}); err == nil {
			t.Fatalf("%s: expected writing-only snapshot to fail, got %q", stage, got)
		} else if !strings.Contains(err.Error(), "prompts.") {
			t.Fatalf("%s: err = %v, want missing current prompt key", stage, err)
		}
	}
}

// TestPromptSnapshotFixPrefersFixKey verifies resume uses the frozen fix prompt.
func TestPromptSnapshotFixPrefersFixKey(t *testing.T) {
	repo := gitRepo(t)
	runID := "fix-key-run"
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), `wo:
  prompts:
    planning: planning
    execution: "execution {{.Stage}}\n"
    review: "review {{.Stage}}\n"
    fix: "fix {{.Stage}} {{.FixSummaryPath}}\n"
    qa: "qa {{.Stage}}\n"
    archive: archive
`)
	mustSnapshotPrompts(t, repo, runID)
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), `wo:
  prompts:
    execution: changed execution
    fix: changed fix
`)

	got, err := promptForStage(repo, State{RunID: runID, Stage: "fix_2", Sealed: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "fix fix_2 ") || strings.Contains(got, "changed") {
		t.Fatalf("fix prompt = %q, want frozen prompts.fix", got)
	}
}

// TestPromptForStageRejectsLegacySnapshotDir verifies historical prompt files are not read.
func TestPromptForStageRejectsLegacySnapshotDir(t *testing.T) {
	repo := t.TempDir()
	runID := "legacy-run"
	mustWritePrompt(t, filepath.Join(runDir(repo, runID), "prompts", "wo-start.md"), "legacy {{.Stage}}\n")

	got, err := promptForStage(repo, State{RunID: runID, Stage: "execution", Sealed: true})
	if err == nil {
		t.Fatalf("expected missing prompt-snapshot.yaml to fail, got %q", got)
	}
	if strings.Contains(got, "legacy") || !strings.Contains(err.Error(), "prompt-snapshot.yaml") {
		t.Fatalf("prompt=%q err=%v, want YAML snapshot failure without legacy prompt", got, err)
	}
}

// TestSealedPromptMissingAllSnapshotsDoesNotReadCurrentYAML verifies missing snapshots fail closed.
func TestSealedPromptMissingAllSnapshotsDoesNotReadCurrentYAML(t *testing.T) {
	repo := t.TempDir()
	runID := "missing-snapshot-run"
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), `wo:
  prompts:
    execution: "current {{.Stage}}\n"
`)

	_, err := promptForStage(repo, State{RunID: runID, Stage: "execution", Sealed: true})
	if err == nil {
		t.Fatal("expected missing prompt snapshot to fail")
	}
	if !strings.Contains(err.Error(), "缺少 prompt 快照") {
		t.Fatalf("err = %v, want missing snapshot error", err)
	}
}

// TestPrintProgressSuppressesDuplicateChecklist verifies repeated status lines do not repeat the block.
func TestPrintProgressSuppressesDuplicateChecklist(t *testing.T) {
	var progress bytes.Buffer
	engine := NewEngine(t.TempDir(), testRegistry(fakeRunner{}))
	engine.Output = &progress
	state := State{
		RunID:  "run-1",
		Status: statusRunning,
		Stage:  "execution",
		Stages: map[string]string{},
	}
	engine.printProgress(state, "resuming")
	engine.printProgress(state, "running")
	if got := strings.Count(progress.String(), "- 写 未知 →"); got != 1 {
		t.Fatalf("checklist count = %d in:\n%s", got, progress.String())
	}
	if strings.Contains(progress.String(), "workflow running") {
		t.Fatalf("progress should not include action noise:\n%s", progress.String())
	}
}

// TestStageProgressWriterAddsSessionID verifies backend progress keeps README-style stage lines.
func TestStageProgressWriterAddsSessionID(t *testing.T) {
	var progress bytes.Buffer
	engine := NewEngine(t.TempDir(), testRegistry(fakeRunner{}))
	engine.Output = &progress
	state := State{
		RunID:    "run-1",
		Status:   statusRunning,
		Stage:    "review_1",
		Stages:   map[string]string{},
		Workflow: DefaultWorkflowConfig(),
	}
	writer := stageProgressWriter{engine: engine, state: &state}
	if _, err := writer.Write([]byte("agent process started: tool=codex pid=123\nagent session started: tool=codex session=t-1\n")); err != nil {
		t.Fatal(err)
	}
	got := progress.String()
	if !strings.Contains(got, "- 审 t-1 →") {
		t.Fatalf("progress missing session id:\n%s", got)
	}
	if strings.Contains(got, "pid=") || strings.Contains(got, "thread=") || strings.Contains(got, "agent process started") || strings.Contains(got, "agent session started") {
		t.Fatalf("raw codex progress leaked:\n%s", got)
	}
}

// TestEngineSubmitCreatesRunAndStartsDetachedWorker verifies human submission does not run foreground stages.
func TestEngineSubmitCreatesRunAndStartsDetachedWorker(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	var started []string
	previous := startDetachedCommand
	startDetachedCommand = func(_ string, runID string) error {
		started = append(started, runID)
		return nil
	}
	t.Cleanup(func() { startDetachedCommand = previous })
	engine := NewEngine(repo, testRegistry(fakeRunner{}))
	var progress bytes.Buffer
	engine.Output = &progress
	if err := engine.Submit(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	if len(started) != 1 {
		t.Fatalf("detached starts = %v, want one run", started)
	}
	state, err := loadState(repo, started[0])
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != statusRunning || state.Stage != "execution" || state.Stages["execution"] != "" {
		t.Fatalf("submitted state = %#v, want queued execution", state)
	}
	if !strings.Contains(progress.String(), "- 写 未知 →") {
		t.Fatalf("submit progress = %q, want start run id", progress.String())
	}
}

// TestInteractiveStartNewRunArchivesExistingRun verifies replacing a stale run stops it from blocking status.
func TestInteractiveStartNewRunArchivesExistingRun(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	oldRunID := "20260508T163350.431416617Z"
	old := State{
		RunID:      oldRunID,
		ChangeName: "demo",
		Status:     statusRunning,
		Stage:      "review_1",
		Sessions:   map[string]string{"codex:executor": "old-executor"},
		Stages:     map[string]string{"execution": "completed"},
		Workflow:   DefaultWorkflowConfig(),
	}
	if err := saveState(repo, old); err != nil {
		t.Fatal(err)
	}
	var startedBatches []string
	previous := startDetachedBatchCommand
	startDetachedBatchCommand = func(_ string, batchID string) error {
		startedBatches = append(startedBatches, batchID)
		return nil
	}
	t.Cleanup(func() { startDetachedBatchCommand = previous })

	engine := NewEngine(repo, testRegistry(fakeRunner{}))
	var stdout bytes.Buffer
	input := strings.NewReader("3\n2\n1\n")
	if err := interactive(context.Background(), input, &stdout, repo, engine); err != nil {
		t.Fatal(err)
	}
	if len(startedBatches) != 1 {
		t.Fatalf("detached batch starts = %v, want one replacement queue", startedBatches)
	}
	archived, err := loadState(repo, oldRunID)
	if err != nil {
		t.Fatal(err)
	}
	if archived.Status != statusArchived {
		t.Fatalf("old status = %s, want archived", archived.Status)
	}
	batch, err := loadBatchState(repo, startedBatches[0])
	if err != nil {
		t.Fatal(err)
	}
	if batch.Status != batchStatusRunning || strings.Join(batch.Changes, ",") != "demo" {
		t.Fatalf("batch = %#v, want replacement single-change queue", batch)
	}
}

// TestEngineResumeDetachedStartsWorker verifies menu resume does not run the workflow in the terminal.
func TestEngineResumeDetachedStartsWorker(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	runID := "resume-detached-run"
	state := State{
		RunID:      runID,
		ChangeName: "demo",
		Status:     statusRunning,
		Stage:      "fix_1",
		Sessions:   map[string]string{"codex:executor": "executor-thread"},
		Stages:     map[string]string{"execution": "completed", "review_1": "completed"},
		Workflow:   DefaultWorkflowConfig(),
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	var started []string
	previous := startDetachedCommand
	startDetachedCommand = func(_ string, runID string) error {
		started = append(started, runID)
		return nil
	}
	t.Cleanup(func() { startDetachedCommand = previous })
	engine := NewEngine(repo, testRegistry(fakeRunner{}))
	var progress bytes.Buffer
	engine.Output = &progress
	if err := engine.ResumeDetachedAfterUserChoice(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(started, ","); got != runID {
		t.Fatalf("detached resume starts = %q, want %s", got, runID)
	}
	if strings.Contains(progress.String(), "pid=") || !strings.Contains(progress.String(), "- 写 executor-thread ✓") || !strings.Contains(progress.String(), "- 修 未知 →") {
		t.Fatalf("resume progress = %q, want compact current stage", progress.String())
	}
	final, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Stage != "fix_1" || final.Stages["fix_1"] != "" {
		t.Fatalf("resume should not run foreground stages: %#v", final)
	}
}

// TestEngineResumeDetachedAfterUserChoiceDoesNotRunForeground verifies menu resume returns after spawning.
func TestEngineResumeDetachedAfterUserChoiceDoesNotRunForeground(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	runID := "resume-detached"
	head, diff, err := gitSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	state := State{
		RunID:        runID,
		ChangeName:   "demo",
		Sealed:       true,
		Status:       statusRunning,
		Stage:        "fix_3",
		BaselineHead: head,
		BaselineDiff: diff,
		Sessions:     map[string]string{"codex:executor": "executor-thread", "codex:reviewer": "reviewer-thread"},
		Stages: map[string]string{
			"execution": "completed",
			"review_1":  "completed",
			"fix_1":     "completed",
			"review_2":  "completed",
			"fix_2":     "completed",
			"review_3":  "completed",
		},
		Workflow: DefaultWorkflowConfig(),
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	var started []string
	previous := startDetachedCommand
	startDetachedCommand = func(_ string, runID string) error {
		started = append(started, runID)
		return nil
	}
	t.Cleanup(func() { startDetachedCommand = previous })
	engine := NewEngine(repo, testRegistry(fakeRunner{}))
	var progress bytes.Buffer
	engine.Output = &progress
	if err := engine.ResumeDetachedAfterUserChoice(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	if len(started) != 1 || started[0] != runID {
		t.Fatalf("detached starts = %v, want %s", started, runID)
	}
	if strings.Contains(progress.String(), "pid=") || !strings.Contains(progress.String(), "- 写 executor-thread ✓") || !strings.Contains(progress.String(), "- 修 未知 ✓✓→") {
		t.Fatalf("resume progress = %q, want compact current fix stage", progress.String())
	}
}

// TestStageChecklistLinesGroupRunningFixBySessionRole verifies repair progress has its own role.
func TestStageChecklistLinesGroupRunningFixBySessionRole(t *testing.T) {
	state := State{
		RunID:    "run-1",
		Status:   statusRunning,
		Stage:    "fix_2",
		Sessions: map[string]string{"codex:executor": "executor-thread", "codex:reviewer": "reviewer-thread"},
		Stages:   map[string]string{"execution": "completed", "review_1": "completed", "fix_1": "completed", "review_2": "completed"},
		Workflow: DefaultWorkflowConfig(),
	}
	lines := strings.Join(stageChecklistLines(state, map[string]stageRuntime{"fix_2": {}}), "\n")
	if !strings.Contains(lines, "- 写 executor-thread ✓") || !strings.Contains(lines, "- 审 reviewer-thread ✓✓") || !strings.Contains(lines, "- 修 未知 ✓→") {
		t.Fatalf("progress should group write/review sessions:\n%s", lines)
	}
}

// TestStageChecklistLinesGroupRunningReviewBySessionRole verifies review progress counts completed reviews.
func TestStageChecklistLinesGroupRunningReviewBySessionRole(t *testing.T) {
	state := State{
		RunID:    "run-1",
		Status:   statusRunning,
		Stage:    "review_3",
		Sessions: map[string]string{"codex:executor": "executor-thread", "codex:reviewer": "reviewer-thread"},
		Stages:   map[string]string{"execution": "completed", "review_1": "completed", "fix_1": "completed", "review_2": "completed", "fix_2": "completed"},
		Workflow: DefaultWorkflowConfig(),
	}
	lines := strings.Join(stageChecklistLines(state, map[string]stageRuntime{"review_3": {}}), "\n")
	if !strings.Contains(lines, "- 写 executor-thread ✓") || !strings.Contains(lines, "- 审 reviewer-thread ✓✓→") || !strings.Contains(lines, "- 修 未知 ✓✓") {
		t.Fatalf("progress should group running review:\n%s", lines)
	}
}

// TestStageChecklistLinesShowUnknownExecutorForFailedExecution verifies terminal execution still counts as occurred.
func TestStageChecklistLinesShowUnknownExecutorForFailedExecution(t *testing.T) {
	state := State{
		RunID:    "run-failed",
		Status:   statusFailed,
		Stage:    "execution",
		Sessions: map[string]string{},
		Stages:   map[string]string{},
		Workflow: DefaultWorkflowConfig(),
	}
	lines := strings.Join(stageChecklistLines(state, nil), "\n")
	if !strings.Contains(lines, "- 写 未知") || strings.Contains(lines, "run-failed") {
		t.Fatalf("failed execution without session should show unknown executor only:\n%s", lines)
	}
}

// TestStageChecklistLinesShowArchiverSession verifies archive progress uses its own role line.
func TestStageChecklistLinesShowArchiverSession(t *testing.T) {
	state := State{
		RunID:    "run-1",
		Status:   statusRunning,
		Stage:    "archive",
		Sessions: map[string]string{"codex:executor": "executor-thread", "codex:reviewer": "reviewer-thread", "codex:archiver": "archiver-thread"},
		Stages:   map[string]string{"execution": "completed", "review_1": "completed"},
		Workflow: DefaultWorkflowConfig(),
	}
	lines := strings.Join(stageChecklistLines(state, map[string]stageRuntime{"archive": {}}), "\n")
	if !strings.Contains(lines, "- 存 archiver-thread →") || strings.Contains(lines, "- 存 executor-thread") {
		t.Fatalf("archive should use archiver session:\n%s", lines)
	}

	state.Status = statusDone
	state.Stage = "done"
	state.Stages["archive"] = "completed"
	lines = strings.Join(stageChecklistLines(state, nil), "\n")
	if !strings.Contains(lines, "- 存 archiver-thread ✓") {
		t.Fatalf("completed archive should show one done marker:\n%s", lines)
	}
	if strings.Contains(lines, "✓→") {
		t.Fatalf("completed archive should not remain marked running:\n%s", lines)
	}
}

// TestStageDurationSummaryLinesFormatsFormula verifies totals and per-stage minutes.
func TestStageDurationSummaryLinesFormatsFormula(t *testing.T) {
	state := State{
		Status:   statusDone,
		Stage:    "done",
		Sessions: map[string]string{"codex:executor": "executor-thread", "codex:reviewer": "reviewer-thread", "codex:archiver": "archiver-thread"},
		Stages:   map[string]string{"execution": "completed", "review_1": "completed", "archive": "completed"},
		StageTimings: map[string]StageTiming{
			"execution": {StartedAt: "2026-05-25T00:00:00Z", FinishedAt: "2026-05-25T00:01:30Z"},
			"review_1":  {StartedAt: "2026-05-25T00:01:30Z", FinishedAt: "2026-05-25T00:02:45Z"},
			"archive":   {StartedAt: "2026-05-25T00:02:45Z", FinishedAt: "2026-05-25T00:03:00Z"},
		},
		Workflow: DefaultWorkflowConfig(),
	}
	lines := strings.Join(stageDurationSummaryLines(state, time.Date(2026, 5, 25, 0, 4, 0, 0, time.UTC)), "\n")
	for _, want := range []string{
		"- 耗时 3分钟=1.5+1.25+0.25",
		"  - 写 execution 1.5分钟",
		"  - 审 review_1 1.25分钟",
		"  - 存 archive 0.25分钟",
	} {
		if !strings.Contains(lines, want) {
			t.Fatalf("duration lines missing %q:\n%s", want, lines)
		}
	}
}

// TestStageDurationSummaryLinesAggregatesReviewAndFixRounds verifies iterated roles are summed.
func TestStageDurationSummaryLinesAggregatesReviewAndFixRounds(t *testing.T) {
	state := State{
		Status: statusDone,
		Stage:  "done",
		Stages: map[string]string{
			"execution": "completed",
			"review_1":  "completed",
			"fix_1":     "completed",
			"review_2":  "completed",
			"fix_2":     "completed",
			"archive":   "completed",
		},
		StageTimings: map[string]StageTiming{
			"execution": {StartedAt: "2026-05-25T00:00:00Z", FinishedAt: "2026-05-25T00:01:00Z"},
			"review_1":  {StartedAt: "2026-05-25T00:01:00Z", FinishedAt: "2026-05-25T00:03:00Z"},
			"fix_1":     {StartedAt: "2026-05-25T00:03:00Z", FinishedAt: "2026-05-25T00:06:00Z"},
			"review_2":  {StartedAt: "2026-05-25T00:06:00Z", FinishedAt: "2026-05-25T00:10:00Z"},
			"fix_2":     {StartedAt: "2026-05-25T00:10:00Z", FinishedAt: "2026-05-25T00:15:00Z"},
			"archive":   {StartedAt: "2026-05-25T00:15:00Z", FinishedAt: "2026-05-25T00:21:00Z"},
		},
		Workflow: DefaultWorkflowConfig(),
	}
	lines := strings.Join(stageDurationSummaryLines(state, time.Date(2026, 5, 25, 0, 22, 0, 0, time.UTC)), "\n")
	for _, want := range []string{
		"- 耗时 21分钟=1+6+8+6",
		"  - 审 review 6分钟",
		"  - 修 fix 8分钟",
	} {
		if !strings.Contains(lines, want) {
			t.Fatalf("aggregated duration lines missing %q:\n%s", want, lines)
		}
	}
	for _, banned := range []string{"review_1", "review_2", "fix_1", "fix_2"} {
		if strings.Contains(lines, banned) {
			t.Fatalf("duration lines must aggregate %s:\n%s", banned, lines)
		}
	}
}

// TestStageDurationSummaryLinesUsesNowForRunningStage verifies live duration display.
func TestStageDurationSummaryLinesUsesNowForRunningStage(t *testing.T) {
	state := State{
		Status:       statusRunning,
		Stage:        "execution",
		Stages:       map[string]string{"execution": "running"},
		StageTimings: map[string]StageTiming{"execution": {StartedAt: "2026-05-25T00:00:00Z"}},
		Workflow:     DefaultWorkflowConfig(),
	}
	now := time.Date(2026, 5, 25, 0, 2, 45, 0, time.UTC)
	lines := strings.Join(stageDurationSummaryLines(state, now), "\n")
	if !strings.Contains(lines, "- 耗时 2.75分钟=2.75") || !strings.Contains(lines, "  - 写 execution 2.75分钟") {
		t.Fatalf("running duration lines:\n%s", lines)
	}
}

// TestStageDurationSummaryLinesSkipsInvalidTimings verifies bad records stay hidden.
func TestStageDurationSummaryLinesSkipsInvalidTimings(t *testing.T) {
	state := State{
		Status: statusDone,
		Stage:  "done",
		Stages: map[string]string{"execution": "completed", "review_1": "completed", "archive": "completed"},
		StageTimings: map[string]StageTiming{
			"execution": {StartedAt: "bad", FinishedAt: "2026-05-25T00:01:00Z"},
			"review_1":  {StartedAt: "2026-05-25T00:03:00Z", FinishedAt: "2026-05-25T00:02:00Z"},
			"archive":   {StartedAt: "2026-05-25T00:00:00Z", FinishedAt: "2026-05-25T00:01:00Z"},
		},
		Workflow: DefaultWorkflowConfig(),
	}
	lines := strings.Join(stageDurationSummaryLines(state, time.Date(2026, 5, 25, 0, 4, 0, 0, time.UTC)), "\n")
	if strings.Contains(lines, "execution") || strings.Contains(lines, "review_1") || !strings.Contains(lines, "- 耗时 1分钟=1") {
		t.Fatalf("invalid timing filtering:\n%s", lines)
	}
}

// TestLegacyAcceptanceStageIsUnknown verifies the state machine no longer keeps the old acceptance stage.
func TestLegacyAcceptanceStageIsUnknown(t *testing.T) {
	engine := &Engine{Repo: t.TempDir()}
	state := State{Stage: "acceptance", Workflow: DefaultWorkflowConfig()}

	if done, err := engine.artifactDone(state); err == nil || done || !strings.Contains(err.Error(), `未知阶段 "acceptance"`) {
		t.Fatalf("artifactDone acceptance = done %v err %v, want unknown stage", done, err)
	}
	if err := engine.advance(&state); err == nil || !strings.Contains(err.Error(), `未知阶段 "acceptance"`) {
		t.Fatalf("advance acceptance err = %v, want unknown stage", err)
	}
}

// TestResumeSkipsExistingExecutionArtifact verifies resume advances when current artifact already exists.
func TestResumeSkipsExistingExecutionArtifact(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	mustSnapshotPrompts(t, repo, "resume-run")
	if err := os.WriteFile(filepath.Join(repo, "docs", "changes", "demo", "task.md"), []byte("- [x] task\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	head, diff, err := gitSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	state := State{
		RunID:        "resume-run",
		ChangeName:   "demo",
		Sealed:       true,
		Status:       statusRunning,
		Stage:        "execution",
		BaselineHead: head,
		BaselineDiff: diff,
		Sessions:     map[string]string{},
		Stages:       map[string]string{"execution": statusInterrupted},
		Workflow:     zeroReviewWorkflow(),
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(repo, testRegistry(fakeRunner{}))
	if err := engine.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	final, err := loadState(repo, "resume-run")
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != statusDone {
		t.Fatalf("status = %s, want done", final.Status)
	}
	if final.Stages["execution"] != "completed" {
		t.Fatalf("execution stage = %q, want completed", final.Stages["execution"])
	}
}

// TestResumeAcceptsManualArtifactCompletion verifies completed artifacts win before intervention checks.
func TestResumeAcceptsManualArtifactCompletion(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	mustSnapshotPrompts(t, repo, "manual-artifact-run")
	head, diff, err := gitSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	runID := "manual-artifact-run"
	state := State{
		RunID:        runID,
		ChangeName:   "demo",
		Sealed:       true,
		Status:       statusRunning,
		Stage:        "execution",
		BaselineHead: head,
		BaselineDiff: diff,
		Sessions:     map[string]string{},
		Stages:       map[string]string{"execution": statusInterrupted},
		Workflow:     zeroReviewWorkflow(),
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", "changes", "demo", "task.md"), []byte("- [x] task\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(repo, testRegistry(fakeRunner{}))
	if err := engine.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	final, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status == statusAborted {
		t.Fatalf("status = %s, want resumable completion", final.Status)
	}
	if final.Stages["execution"] != "completed" {
		t.Fatalf("execution stage = %q, want completed", final.Stages["execution"])
	}
}

// TestResumeMarksInterruptedStageCompletedWhenArtifactExists verifies artifact-based resume fixes stale stage status.
func TestResumeMarksInterruptedStageCompletedWhenArtifactExists(t *testing.T) {
	cases := []struct {
		name  string
		stage string
		setup func(t *testing.T, repo, runID string)
	}{
		{
			name:  "review",
			stage: "review_1",
			setup: func(t *testing.T, repo, runID string) {
				t.Helper()
				mustWritePrompt(t, filepath.Join(runDir(repo, runID), "review-1.json"), cleanReviewJSON())
			},
		},
		{
			name:  "fix",
			stage: "fix_1",
			setup: func(t *testing.T, repo, runID string) {
				t.Helper()
				mustWritePrompt(t, filepath.Join(runDir(repo, runID), "fix-1-summary.md"), "done\n")
			},
		},
		{
			name:  "archive",
			stage: "archive",
			setup: func(t *testing.T, repo, runID string) {
				t.Helper()
				mustWritePrompt(t, filepath.Join(runDir(repo, runID), "review-1.json"), cleanReviewJSON())
				mustWritePrompt(t, filepath.Join(runDir(repo, runID), "qa-1.json"), cleanQAJSON())
				mustWritePrompt(t, filepath.Join(runDir(repo, runID), "delivery-summary.md"), "done\n")
				if err := os.MkdirAll(filepath.Join(repo, "docs", "changes", "archive", "2026-05-05-demo"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := gitRepo(t)
			runID := "resume-run-" + tc.name
			mustChange(t, repo, "demo")
			mustPrompts(t, repo)
			mustSnapshotPrompts(t, repo, runID)
			mustWritePrompt(t, filepath.Join(runDir(repo, runID), "acceptance.json"), acceptanceJSON())
			tc.setup(t, repo, runID)
			head, diff, err := gitSnapshot(repo)
			if err != nil {
				t.Fatal(err)
			}
			stages := map[string]string{tc.stage: statusInterrupted}
			if tc.stage == "archive" {
				stages["review_1"] = "completed"
				stages["qa_1"] = "completed"
			}
			state := State{
				RunID:        runID,
				ChangeName:   "demo",
				Sealed:       true,
				Status:       statusRunning,
				Stage:        tc.stage,
				BaselineHead: head,
				BaselineDiff: diff,
				Sessions:     map[string]string{},
				Stages:       stages,
				Workflow:     DefaultWorkflowConfig(),
			}
			if err := saveState(repo, state); err != nil {
				t.Fatal(err)
			}
			engine := NewEngine(repo, testRegistry(fakeRunner{}))
			if err := engine.Resume(context.Background()); err != nil {
				t.Fatal(err)
			}
			final, err := loadState(repo, runID)
			if err != nil {
				t.Fatal(err)
			}
			if final.Stages[tc.stage] != "completed" {
				t.Fatalf("%s stage = %q, want completed", tc.stage, final.Stages[tc.stage])
			}
		})
	}
}

// TestPromptForStageReadsExactYAMLPrompt verifies stage prompts are injected without decoration.
func TestPromptForStageReadsExactPromptFile(t *testing.T) {
	repo := t.TempDir()
	want := "custom prompt\nwith exact bytes\n"
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  prompts:\n    execution: |\n      custom prompt\n      with exact bytes\n")
	got, err := promptForStage(repo, State{Stage: "execution"})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}
}

// TestPromptForStageReadsYAMLPromptKeys verifies sealed stages use the public prompt keys.
func TestPromptForStageReadsREADMETemplateNames(t *testing.T) {
	repo := t.TempDir()
	cases := []struct {
		stage string
		key   string
	}{
		{stage: "execution", key: "execution"},
		{stage: "review_1", key: "review"},
		{stage: "fix_1", key: "fix"},
		{stage: "archive", key: "archive"},
	}
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  prompts:\n    execution: \"execution {{.Stage}}\\n\"\n    review: \"review {{.Stage}}\\n\"\n    fix: \"fix {{.Stage}}\\n\"\n    archive: \"archive {{.Stage}}\\n\"\n")
	for _, tc := range cases {
		got, err := promptForStage(repo, State{Stage: tc.stage})
		if err != nil {
			t.Fatalf("%s: %v", tc.stage, err)
		}
		want := tc.key + " " + tc.stage + "\n"
		if got != want {
			t.Fatalf("%s prompt = %q, want %q", tc.stage, got, want)
		}
	}
}

// TestPromptForStagePrefersRepoYAMLPrompt verifies repository prompts override global prompts.
func TestPromptForStagePrefersLocalPrompt(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	mustWritePrompt(t, filepath.Join(home, "wo.yaml"), "wo:\n  prompts:\n    execution: \"global\\n\"\n")
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  prompts:\n    execution: \"local\\n\"\n")
	mustWritePrompt(t, filepath.Join(repo, ".wo", "cmd", "wo-start.md"), "legacy\n")
	got, err := promptForStage(repo, State{Stage: "execution"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "local\n" {
		t.Fatalf("prompt = %q, want local prompt", got)
	}
}

// TestPromptForStageFallsBackToGlobalPrompt verifies ~/wo.yaml is used when local prompt is absent.
func TestPromptForStageFallsBackToGlobalPrompt(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	mustWritePrompt(t, filepath.Join(home, "wo.yaml"), "wo:\n  prompts:\n    execution: \"global\\n\"\n")
	got, err := promptForStage(repo, State{Stage: "execution"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "global\n" {
		t.Fatalf("prompt = %q, want global prompt", got)
	}
}

// TestPlanningPromptReadsDiscussTemplate verifies interactive planning uses the README template name.
func TestPlanningPromptReadsDiscussTemplate(t *testing.T) {
	repo := t.TempDir()
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  prompts:\n    planning: \"stage={{.Stage}} max={{.MaxReviewIterations}}\\n\"\n")
	got, options, err := planningPrompt(repo)
	if err != nil {
		t.Fatal(err)
	}
	if got != "stage=planning max=5\n" {
		t.Fatalf("prompt = %q, want rendered wo-discuss prompt", got)
	}
	if options.Tool != "codex" || options.Reasoning != "xhigh" || !options.Fast {
		t.Fatalf("planning options = %#v, want codex xhigh with fast enabled", options)
	}
}

// TestPlanningPromptCarriesPlanningContext verifies interactive planning consumes enabled planning helpers.
func TestPlanningPromptCarriesPlanningContext(t *testing.T) {
	repo := t.TempDir()
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  workflow:\n    parallel:\n      enabled: true\n")
	got, _, err := planningPrompt(repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"parallel-planning-context.json", "planning_context", "advisory 成员失败"} {
		if !strings.Contains(got, want) {
			t.Fatalf("planning prompt missing %q:\n%s", want, got)
		}
	}
}

// TestPlanningSessionIDIsStoredOnCreatedRun verifies status can show planning sessions.
func TestPlanningSessionIDIsStoredOnCreatedRun(t *testing.T) {
	repo := gitRepo(t)
	mustPrompts(t, repo)
	mustChange(t, repo, "demo")
	installFakeOz(t, "demo")
	engine := NewEngine(repo, testRegistry(fakeRunner{}))
	engine.PlanningTool = "codex"
	engine.PlanningSessionID = "plan-123"

	state, err := engine.createRun("demo")
	if err != nil {
		t.Fatal(err)
	}
	if got := state.Sessions[sessionStateKey("codex", "planner")]; got != "plan-123" {
		t.Fatalf("planning session = %q, want plan-123", got)
	}
	lines := strings.Join(stageChecklistLines(state, nil), "\n")
	if !strings.Contains(lines, "- 规 plan-123") {
		t.Fatalf("status lines missing planning session:\n%s", lines)
	}
}

// TestPlanningSessionIDFromFile extracts the TUI-safe planning side channel.
func TestPlanningSessionIDFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "planning-session")
	if err := os.WriteFile(path, []byte("plan-123\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := planningSessionIDFromFile(path); got != "plan-123" {
		t.Fatalf("planning session = %q, want plan-123", got)
	}
}

// TestPromptNameForStageMapsDynamicStages verifies sealed stages use named templates.
func TestPromptNameForStageMapsDynamicStages(t *testing.T) {
	stages := map[string]string{
		"planning":  "wo-discuss",
		"execution": "wo-start",
		"review_1":  "wo-review",
		"fix_1":     "wo-fix",
		"review_9":  "wo-review",
		"fix_9":     "wo-fix",
		"archive":   "wo-done",
	}
	for stage, want := range stages {
		got, err := promptNameForStage(stage)
		if err != nil {
			t.Fatalf("%s: %v", stage, err)
		}
		if got != want {
			t.Fatalf("%s prompt = %s, want %s", stage, got, want)
		}
	}
}

// TestWorkflowRoleMappingsComeFromRoleDefinitions verifies prompt, session, and labels stay aligned.
func TestWorkflowRoleMappingsComeFromRoleDefinitions(t *testing.T) {
	cases := []struct {
		stage     string
		promptKey string
		session   string
		label     string
	}{
		{stage: "planning", promptKey: "planning", session: "planner", label: "规"},
		{stage: "execution", promptKey: "execution", session: "executor", label: "写"},
		{stage: "review_3", promptKey: "review", session: "reviewer", label: "审"},
		{stage: "fix_3", promptKey: "fix", session: "fixer", label: "修"},
		{stage: "archive", promptKey: "archive", session: "archiver", label: "存"},
	}
	for _, tc := range cases {
		name, err := promptNameForStage(tc.stage)
		if err != nil {
			t.Fatalf("%s prompt name: %v", tc.stage, err)
		}
		key, err := promptKeyForName(name)
		if err != nil {
			t.Fatalf("%s prompt key: %v", tc.stage, err)
		}
		if key != tc.promptKey {
			t.Fatalf("%s prompt key = %s, want %s", tc.stage, key, tc.promptKey)
		}
		if role := stageSessionRole(tc.stage); role != tc.session {
			t.Fatalf("%s session role = %s, want %s", tc.stage, role, tc.session)
		}
		if label := sessionRoleLabel(tc.session); label != tc.label {
			t.Fatalf("%s label = %s, want %s", tc.stage, label, tc.label)
		}
	}
	if stageSessionRole("execution") == stageSessionRole("fix_1") {
		t.Fatal("execution and fix must not share a session role")
	}
	executionName, _ := promptNameForStage("execution")
	fixName, _ := promptNameForStage("fix_1")
	executionKey, _ := promptKeyForName(executionName)
	fixKey, _ := promptKeyForName(fixName)
	if executionKey == fixKey {
		t.Fatal("execution and fix must not share a prompt key")
	}
}

// TestDefaultFixPromptCarriesCurrentReviewContract verifies fix turns expose the review artifact.
func TestDefaultFixPromptCarriesCurrentReviewContract(t *testing.T) {
	repo := gitRepo(t)
	t.Setenv("HOME", t.TempDir())
	runID := "fix-prompt-contract-run"
	mustSnapshotPrompts(t, repo, runID)

	got, err := promptForStage(repo, State{RunID: runID, Stage: "fix_1", Sealed: true, Workflow: DefaultWorkflowConfig()})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"review-1.json", "fix-1-summary.md", "qa-1.json", "只修复当前 review/QA artifact 中列出的 findings"} {
		if !strings.Contains(got, want) {
			t.Fatalf("fix prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "# wo start") {
		t.Fatalf("fix prompt used execution template:\n%s", got)
	}
}

// TestDefaultExecutionPromptKeepsFullFirstTurnContract verifies execution keeps the full oz-exec contract.
func TestDefaultExecutionPromptKeepsFullFirstTurnContract(t *testing.T) {
	repo := gitRepo(t)
	t.Setenv("HOME", t.TempDir())
	runID := "execution-prompt-contract-run"
	mustSnapshotPrompts(t, repo, runID)

	got, err := promptForStage(repo, State{RunID: runID, Stage: "execution", Sealed: true, Workflow: DefaultWorkflowConfig()})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"review-1.json", "fix-1-summary.md", "只修复当前 review/QA artifact 中列出的 findings"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("execution prompt should not contain %q:\n%s", forbidden, got)
		}
	}
	if !strings.Contains(got, "oz-exec") {
		t.Fatalf("execution prompt should reference oz-exec:\n%s", got)
	}
	for _, want := range []string{
		"proposal.md",
		"design.md",
		"spec.md",
		"task.md",
		"acceptance.json",
		"required_tests",
		"不得删除、弱化、跳过或改写",
		"oz status",
		"tasks.done",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("execution prompt missing full first-turn contract %q:\n%s", want, got)
		}
	}
}

// TestParallelEnabledPromptsCarryFanoutArtifacts verifies optional helpers enter stage prompts only when enabled.
func TestParallelEnabledPromptsCarryFanoutArtifacts(t *testing.T) {
	repo := gitRepo(t)
	workflow := DefaultWorkflowConfig()
	workflow.Parallel.Enabled = true
	state := State{ChangeName: "demo", Sealed: true, Workflow: workflow}

	cases := []struct {
		stage string
		want  []string
	}{
		{stage: "planning", want: []string{"parallel-planning-context.json", "planning_context", "advisory 成员失败"}},
		{stage: "execution", want: []string{"parallel-planning-context.json", "parallel-implementation-context.json", "implementation_context", "advisory 成员失败"}},
		{stage: "review_1", want: []string{"parallel-planning-context.json", "parallel-implementation-context.json", "parallel-review-1.json", "gate_input 成员报告 blocker/major"}},
		{stage: "qa_1", want: []string{"parallel-planning-context.json", "parallel-implementation-context.json", "parallel-qa-1.json", "acceptance_matrix"}},
	}
	for _, tc := range cases {
		t.Run(tc.stage, func(t *testing.T) {
			state.Stage = tc.stage
			got, err := promptForStage(repo, state)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Fatalf("%s prompt missing %q:\n%s", tc.stage, want, got)
				}
			}
		})
	}
}

// TestPromptContextExposesRoleSessionTurnState verifies prompt templates can branch on resumed role sessions.
func TestPromptContextExposesRoleSessionTurnState(t *testing.T) {
	repo := t.TempDir()
	workflow := DefaultWorkflowConfig()
	workflow.Stages["review_2"] = StageOptions{Tool: "opencode"}
	state := State{
		RunID:      "run",
		ChangeName: "demo",
		Stage:      "review_2",
		Workflow:   workflow,
		Sessions: map[string]string{
			"codex:reviewer":    "wrong-tool-session",
			"opencode:reviewer": "review-session",
		},
	}

	context := promptContext(repo, state)
	if context.RoleSessionKey != "opencode:reviewer" || context.RoleSessionID != "review-session" {
		t.Fatalf("role session = %q/%q, want opencode reviewer session", context.RoleSessionKey, context.RoleSessionID)
	}
	if !context.HasRoleSession || context.IsFirstRoleTurn {
		t.Fatalf("turn flags = has:%t first:%t, want resumed role turn", context.HasRoleSession, context.IsFirstRoleTurn)
	}

	state.Stage = "qa_2"
	context = promptContext(repo, state)
	if context.RoleSessionKey != "codex:qa" || context.RoleSessionID != "" {
		t.Fatalf("qa role session = %q/%q, want isolated empty codex qa session", context.RoleSessionKey, context.RoleSessionID)
	}
	if context.HasRoleSession || !context.IsFirstRoleTurn {
		t.Fatalf("qa turn flags = has:%t first:%t, want first role turn", context.HasRoleSession, context.IsFirstRoleTurn)
	}
}

// TestBundledFixPromptRequiresRootCauseAnalysis verifies fix prompts discourage patch-only loops.
func TestBundledFixPromptRequiresRootCauseAnalysis(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "prompts-template", "wo-fix.md"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := renderPromptTemplate("wo-fix", string(data), promptTemplateContext{
		Stage:                     "fix_2",
		Iteration:                 2,
		IsFirstRoleTurn:           true,
		MaxReviewIterations:       3,
		StatePath:                 ".wo/runs/run/state.json",
		ReviewPath:                ".wo/runs/run/review-2.json",
		FixSummaryPath:            ".wo/runs/run/fix-2-summary.md",
		FixEscalated:              true,
		FixEscalationReasoning:    "medium",
		ConsecutiveReviewFailures: 4,
		RepeatedFindingTitles:     []string{"Playwright WebUI cannot enter chat surface"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"升级轮次", "根因分析", "禁止只按错误文本打补丁", "上一轮未解决原因"} {
		if !strings.Contains(got, want) {
			t.Fatalf("fix prompt missing %q:\n%s", want, got)
		}
	}
}

// TestBundledRolePromptsTrimExamplesAfterSessionResume verifies resumed role sessions avoid replaying startup examples.
func TestBundledRolePromptsTrimExamplesAfterSessionResume(t *testing.T) {
	repo := gitRepo(t)
	runID := "role-prompt-trim-run"
	mustSnapshotPrompts(t, repo, runID)
	cases := []struct {
		stage    string
		sessions map[string]string
		rejects  []string
	}{
		{stage: "review_2", sessions: map[string]string{"codex:reviewer": "review-session"}, rejects: []string{"JSON schema：", "如需修复，使用：", "\"decision\": \"needs_fix\""}},
		{stage: "qa_2", sessions: map[string]string{"codex:qa": "qa-session"}, rejects: []string{"clean 示例：", "needs_fix 示例：", "\"summary\": \"核心业务路径已通过 QA\""}},
		{stage: "fix_2", sessions: map[string]string{"codex:fixer": "fix-session"}, rejects: []string{"充分理解评审意见", "从根源入手，不能治标不治本", "禁止只按错误文本打补丁"}},
	}
	for _, tc := range cases {
		t.Run(tc.stage, func(t *testing.T) {
			got, err := promptForStage(repo, State{RunID: runID, ChangeName: "demo", Stage: tc.stage, Sealed: true, Workflow: DefaultWorkflowConfig(), Sessions: tc.sessions})
			if err != nil {
				t.Fatal(err)
			}
			for _, reject := range tc.rejects {
				if strings.Contains(got, reject) {
					t.Fatalf("%s prompt repeated %q:\n%s", tc.stage, reject, got)
				}
			}
		})
	}
}

// TestBundledReviewPromptUsesLatestHistoryOnly verifies multi-round reviews stay focused.
func TestBundledReviewPromptUsesLatestHistoryOnly(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "prompts-template", "wo-review.md"))
	if err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	state := State{
		RunID:      "run",
		ChangeName: "demo",
		Stage:      "review_5",
		Workflow:   DefaultWorkflowConfig(),
		Stages:     map[string]string{},
	}
	context := promptContext(repo, state)
	got, err := renderPromptTemplate("wo-review", string(data), context)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"完整变更", "docs/changes/demo", "review-5.json", "review-4.json", "fix-4-summary.md", "历史 review 数量：4", "严格 JSON", "evidence 必须可复核", "workflow_failure"} {
		if !strings.Contains(got, want) {
			t.Fatalf("review prompt missing %q:\n%s", want, got)
		}
	}
	for _, reject := range []string{"review-1.json", "review-2.json", "review-3.json", "fix-1-summary.md", "fix-2-summary.md", "fix-3-summary.md", "Previous reviews:", "Previous fixes:"} {
		if strings.Contains(got, reject) {
			t.Fatalf("review prompt listed old history %q:\n%s", reject, got)
		}
	}
}

// TestBundledFixPromptFocusesCurrentReview verifies normal fixes do not replay history.
func TestBundledFixPromptFocusesCurrentReview(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "prompts-template", "wo-fix.md"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := renderPromptTemplate("wo-fix", string(data), promptTemplateContext{
		Stage:               "fix_2",
		Iteration:           2,
		MaxReviewIterations: 3,
		StatePath:           ".wo/runs/run/state.json",
		ReviewPath:          ".wo/runs/run/review-2.json",
		FixSummaryPath:      ".wo/runs/run/fix-2-summary.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"review-2.json", "fix-2-summary.md", "只修复当前 review/QA artifact 中列出的 findings"} {
		if !strings.Contains(got, want) {
			t.Fatalf("fix prompt missing %q:\n%s", want, got)
		}
	}
	for _, reject := range []string{"review-1.json", "fix-1-summary.md", "自动升级", "普通修复轮次不需要读取所有旧 review/fix artifact"} {
		if strings.Contains(got, reject) {
			t.Fatalf("normal fix prompt included escalation/history %q:\n%s", reject, got)
		}
	}
}

// TestBundledOzSkillPromptsDelegateToSkills verifies agent prompts do not duplicate oz skill bodies.
func TestBundledOzSkillPromptsDelegateToSkills(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mustHave   []string
		mustReject []string
	}{
		{
			name:       "wo-discuss",
			mustHave:   []string{"oz-plan", "讨论规划阶段"},
			mustReject: []string{"change-name", "open questions"},
		},
		{
			name:       "wo-start",
			mustHave:   []string{"oz-exec", "state.json.change_name", "当前 oz change", "proposal.md", "acceptance.json", "required_tests", "oz status", "tasks.done"},
			mustReject: []string{"review-1.json", "fix-1-summary.md", "只修复当前 review/QA artifact 中列出的 findings"},
		},
		{
			name:       "wo-done",
			mustHave:   []string{"oz-archive", "delivery-summary.md", "git commit"},
			mustReject: []string{"oz status", "oz validate", "oz archive", "--yes", "tasks.total", "tasks.done", "delta specs"},
		},
	} {
		data, err := os.ReadFile(filepath.Join("..", "..", "prompts-template", tc.name+".md"))
		if err != nil {
			t.Fatal(err)
		}
		body := string(data)
		for _, want := range tc.mustHave {
			if !strings.Contains(body, want) {
				t.Fatalf("%s prompt missing %q:\n%s", tc.name, want, body)
			}
		}
		for _, reject := range tc.mustReject {
			if strings.Contains(body, reject) {
				t.Fatalf("%s prompt still contains %q:\n%s", tc.name, reject, body)
			}
		}
	}
}

// gitRepo creates a temporary repository with one initial commit.
func gitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "init")
	return repo
}

// runGit runs git commands in a test repository.
func runGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

// gitHead reads the current HEAD for test state setup.
func gitHead(t *testing.T, repo string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

// newestRun returns the only run id created by a test.
func newestRun(repo string) (string, error) {
	root, err := runsRoot(repo)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	return entries[len(entries)-1].Name(), nil
}

// currentRunID returns the only active run id for fake runner artifact paths.
func currentRunID(repo string) string {
	root, err := runsRoot(repo)
	if err != nil {
		return ""
	}
	entries, _ := os.ReadDir(root)
	if len(entries) == 0 {
		return ""
	}
	return entries[len(entries)-1].Name()
}

// cleanReviewJSON returns a valid strict review artifact for test stages.
func cleanReviewJSON() string {
	return `{"summary":"ok","decision":"clean","checks":{"oz_aligned":true,"tasks_verified":true,"tests_meaningful":true,"implementation_scoped":true,"runtime_behavior_verified":true,"previous_findings_resolved":true},"evidence":["validation artifact passed: validation-execution-1.json","runtime evidence: Playwright screenshot test-results/demo.png"],"findings":[]}` + "\n"
}

// acceptanceJSON returns a valid strict acceptance contract for test stages.
func acceptanceJSON() string {
	return `{"summary":"demo acceptance","required_tests":[{"id":"contract-demo","source":"change_contract","path":"docs/changes/demo/tests/demo.acceptance.test.ts","command":"pnpm exec tsx --test docs/changes/demo/tests/demo.acceptance.test.ts","purpose":"cover the demo contract"}],"required_evidence":[{"id":"screenshot-demo","kind":"screenshot","path":"test-results/demo.png","purpose":"prove the demo runtime path"}]}` + "\n"
}

// fixReviewJSON returns a valid strict needs_fix artifact for test stages.
func fixReviewJSON() string {
	return `{"summary":"fix","decision":"needs_fix","checks":{"oz_aligned":false,"tasks_verified":false,"tests_meaningful":false,"implementation_scoped":true,"runtime_behavior_verified":false,"previous_findings_resolved":false},"evidence":[],"findings":[{"title":"bug","severity":"major","evidence":"test failed","recommendation":"fix it"}]}` + "\n"
}

// cleanQAJSON returns a valid strict QA artifact for test stages.
func cleanQAJSON() string {
	return `{"summary":"qa ok","decision":"clean","evidence":["Playwright test passed with screenshot artifact test-results/demo.png"],"findings":[],"acceptance_matrix":[{"id":"contract-demo","status":"passed","artifact":"docs/changes/demo/tests/demo.acceptance.test.ts","evidence":"contract test passed"},{"id":"screenshot-demo","status":"passed","artifact":"test-results/demo.png","evidence":"screenshot artifact shows the demo runtime path"}]}` + "\n"
}

// cleanQAWithUnknownAcceptanceIDJSON returns a clean QA artifact that fails the acceptance matrix gate.
func cleanQAWithUnknownAcceptanceIDJSON() string {
	return `{"summary":"qa ok","decision":"clean","evidence":["Playwright test passed with screenshot artifact test-results/demo.png"],"findings":[],"acceptance_matrix":[{"id":"contract-demo","status":"passed","artifact":"docs/changes/demo/tests/demo.acceptance.test.ts","evidence":"contract test passed"},{"id":"screenshot-demo","status":"passed","artifact":"test-results/demo.png","evidence":"screenshot artifact shows the demo runtime path"},{"id":"not-in-acceptance","status":"passed","artifact":"test-results/extra.png","evidence":"extra evidence not listed in acceptance"}]}` + "\n"
}

// fixQAJSON returns a valid strict QA artifact that sends the workflow back to fix.
func fixQAJSON() string {
	return `{"summary":"qa fix","decision":"needs_fix","evidence":["Playwright trace test-results/demo.zip failed"],"findings":[{"title":"qa bug","severity":"major","evidence":"trace shows broken runtime state","recommendation":"fix runtime state and rerun QA"}]}` + "\n"
}

// workflowFailureReviewJSON returns a strict review artifact that terminates the run.
func workflowFailureReviewJSON() string {
	return `{"summary":"连续两轮没有实质变化","decision":"needs_fix","workflow_failure":{"failed":true,"reason":"连续两轮没有实质变化；上游接口缺少必要凭据"},"checks":{"oz_aligned":false,"tasks_verified":false,"tests_meaningful":false,"implementation_scoped":true,"runtime_behavior_verified":false,"previous_findings_resolved":false},"evidence":["fix-1-summary.md 报告缺少必要凭据"],"findings":[{"title":"无法自动完成修复","severity":"blocker","evidence":"连续两轮没有实质变化，且 fix summary 报告缺少必要凭据","recommendation":"停止自动循环，补齐凭据后重启"}]}` + "\n"
}

// mustPrompts creates exact prompt files for every currently automated stage.
func mustPrompts(t *testing.T, repo string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), `wo:
  prompts:
    planning: planning
    execution: "{{.Stage}}"
    review: "{{.Stage}}"
    qa: "{{.Stage}}"
    fix: "{{.Stage}}"
    archive: "{{.Stage}}"
`)
	mustWritePrompt(t, filepath.Join(home, "wo.yaml"), `wo:
  prompts:
    planning: planning
    execution: "{{.Stage}}"
    review: "{{.Stage}}"
    qa: "{{.Stage}}"
    fix: "{{.Stage}}"
    archive: "{{.Stage}}"
`)
}

// mustSnapshotPrompts creates the sealed-run prompt snapshot required by resume.
func mustSnapshotPrompts(t *testing.T, repo, runID string) {
	t.Helper()
	if err := snapshotRunPrompts(repo, runID); err != nil {
		t.Fatal(err)
	}
}

// mustWritePrompt writes a prompt file and creates parent directories.
func mustWritePrompt(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
