// Package app tests go-dag execution context scheduling around completed oz tasks.
package app

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type goDAGContextFakeTool struct {
	runner AgentRunner
}

func (goDAGContextFakeTool) Name() string { return "codex" }

func (goDAGContextFakeTool) Resolve() error { return nil }

func (goDAGContextFakeTool) PlanningCommand(context.Context, string, string, io.Reader, StageOptions) (*exec.Cmd, error) {
	return nil, nil
}

func (t goDAGContextFakeTool) NewRunner() AgentRunner { return t.runner }

type goDAGContextFakeRunner struct {
	mainCalls int
	capture   *artifactCapture
}

// TestGoDAGRetryableHelperErrorRestoresRunningState verifies transient helper failures stay retryable.
func TestGoDAGRetryableHelperErrorRestoresRunningState(t *testing.T) {
	repo := t.TempDir()
	runID := "retryable-helper-run"
	state := State{
		RunID:      runID,
		ChangeName: "demo",
		Status:     statusFailed,
		Stage:      "qa_1",
		Error:      "transient helper failure",
		Sessions:   map[string]string{},
		Stages:     map[string]string{"qa_1": statusRunning},
		Paths:      map[string]string{},
		Workflow:   DefaultWorkflowConfig(),
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(repo, nil)
	node := WorkflowNode{ID: "before_qa_1_4", Type: "subagent", Stage: "qa_1", Group: "qa", Member: "回归场景测试员"}

	if !engine.goDAGShouldRetryNode(runID, node, errors.Join(errGoDAGRetryableNode, errors.New("temporary"))) {
		t.Fatal("retryable helper error should request a go-dag retry")
	}
	restored, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Status != statusRunning || restored.Stage != "qa_1" || restored.Error != "" {
		t.Fatalf("restored state = status %q stage %q error %q, want running qa_1 empty error", restored.Status, restored.Stage, restored.Error)
	}
}

func (r *goDAGContextFakeRunner) SetArtifactCapture(capture *artifactCapture) {
	r.capture = capture
}

// Run writes either a subagent artifact or the execution task completion marker.
func (r *goDAGContextFakeRunner) Run(_ context.Context, repo, prompt, threadID string, _ StageOptions) (string, error) {
	// SUBAGENT_NAME marks helper prompts; main execution prompts update task.md.
	if name := goDAGContextPromptValue(prompt, "SUBAGENT_NAME"); name != "" {
		purpose := goDAGContextPromptValue(prompt, "SUBAGENT_PURPOSE")
		changeName := goDAGContextPromptValue(prompt, "CURRENT_CHANGE")
		body := `{"name":"` + name + `","change_name":"` + changeName + `","purpose":"` + purpose + `","status":"success","summary":"context ready","evidence":["unit-test"]}` + "\n"
		if r.capture != nil {
			r.capture.Append(body)
		}
		return "subagent-" + name, nil
	}
	r.mainCalls++
	task := filepath.Join(repo, "docs", "changes", "demo", "task.md")
	return "executor-thread", os.WriteFile(task, []byte("- [x] task\n"), 0o644)
}

// goDAGContextPromptValue reads key=value metadata from generated prompts.
func goDAGContextPromptValue(prompt, key string) string {
	prefix := key + "="
	for _, line := range strings.Split(prompt, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

// TestGoDAGSkipsExecutionContextWhenTasksAlreadyDone verifies completed changes skip execution helpers.
func TestGoDAGSkipsExecutionContextWhenTasksAlreadyDone(t *testing.T) {
	repo := goDAGContextRepo(t)
	goDAGContextChange(t, repo, "- [x] task\n")
	goDAGContextInstallFakeOz(t)
	runID := "done-execution-context-run"
	if err := snapshotRunPrompts(repo, runID); err != nil {
		t.Fatal(err)
	}
	state := goDAGContextState(t, repo, runID)
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	runner := &goDAGContextFakeRunner{}
	engine := NewEngine(repo, goDAGContextRegistry(runner))

	for _, node := range []WorkflowNode{
		{ID: "implementation_context_1", Type: "subagent", Group: "before_execution", Stage: "execution", Member: "代码库侦察员"},
		{ID: "before_execution_fanin", Type: "fanin", Group: "before_execution", Stage: "execution"},
		{ID: "execution", Type: "main_stage", Stage: "execution"},
	} {
		if err := engine.runGoDAGNode(context.Background(), runID, node); err != nil {
			t.Fatal(err)
		}
	}

	persisted, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if runner.mainCalls != 0 {
		t.Fatalf("main calls = %d, want execution skipped", runner.mainCalls)
	}
	if persisted.Stage != "archive" || persisted.Stages["execution"] != "completed" {
		t.Fatalf("state = stage %q execution %q, want archive/completed", persisted.Stage, persisted.Stages["execution"])
	}
	if len(persisted.Sessions) != 0 {
		t.Fatalf("sessions = %#v, want no subagent or executor sessions", persisted.Sessions)
	}
	if fileExists(parallelArtifactPath(runDir(repo, runID), "implementation_context", 0)) {
		t.Fatal("implementation context fan-in artifact should not exist")
	}
}

// TestGoDAGCompletedExecutionStillRunsValidation proves skipped execution agents do not skip deterministic gates.
func TestGoDAGCompletedExecutionStillRunsValidation(t *testing.T) {
	repo := goDAGContextRepo(t)
	goDAGContextChange(t, repo, "- [x] task\n")
	goDAGContextInstallFakeOz(t)
	runID := "done-execution-validation-run"
	if err := snapshotRunPrompts(repo, runID); err != nil {
		t.Fatal(err)
	}
	state := goDAGContextState(t, repo, runID)
	state.Workflow.Validation.Commands = []ValidationCommand{{
		Executable: "/bin/sh",
		Args:       []string{"-c", "printf validation-ran; exit 7"},
	}}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	runner := &goDAGContextFakeRunner{}
	engine := NewEngine(repo, goDAGContextRegistry(runner))

	err := engine.runGoDAGNode(context.Background(), runID, WorkflowNode{ID: "execution", Type: "main_stage", Stage: "execution"})
	if !errors.Is(err, errGoDAGValidationRetry) {
		t.Fatalf("execution node error = %v, want validation retry", err)
	}
	persisted, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if runner.mainCalls != 0 {
		t.Fatalf("main calls = %d, want execution skipped", runner.mainCalls)
	}
	if persisted.Stage != "execution" || persisted.Stages["execution"] != "validation_failed" {
		t.Fatalf("state = stage %q execution %q, want execution/validation_failed", persisted.Stage, persisted.Stages["execution"])
	}
	validation := persisted.Validation["execution"]
	if validation.Kind != validationKindCommands || validation.Status != validationStatusFailed {
		t.Fatalf("validation = %#v, want failed command validation", validation)
	}
	if validation.LastArtifact == "" || !fileExists(validation.LastArtifact) {
		t.Fatalf("validation artifact = %q, want written artifact", validation.LastArtifact)
	}
}

// TestGoDAGArtifactDoneAllowsSkippedImplementationContext keeps execution gates aligned with skipped helper nodes.
func TestGoDAGArtifactDoneAllowsSkippedImplementationContext(t *testing.T) {
	repo := goDAGContextRepo(t)
	goDAGContextChange(t, repo, "- [x] task\n")
	goDAGContextInstallFakeOz(t)
	runID := "done-execution-artifact-gate-run"
	if err := snapshotRunPrompts(repo, runID); err != nil {
		t.Fatal(err)
	}
	state := goDAGContextState(t, repo, runID)
	state.DAGNodes = map[string]DAGNodeState{
		"execution": {Status: "running"},
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}

	done, err := (&Engine{Repo: repo}).artifactDone(state)
	if err != nil || !done {
		t.Fatalf("artifactDone should allow skipped go-dag implementation context, done=%v err=%v", done, err)
	}
}

// TestWorkflowSpecOrdersImplementationContextBeforeExecution protects subagent read-only boundaries.
func TestWorkflowSpecOrdersImplementationContextBeforeExecution(t *testing.T) {
	workflow := DefaultWorkflowConfig()
	workflow.Engine = "go-dag"
	workflow.Parallel = ParallelConfig{
		Enabled: true,
		Groups: map[string]ParallelGroupConfig{
			"implementation_context": {
				Mode: "advisory",
				Members: []ParallelMemberConfig{
					{Name: "代码库侦察员", Purpose: "汇总 execution 需要读取的文件和测试模式", Tool: "codex"},
				},
			},
		},
	}

	spec := BuildWorkflowSpec("demo", workflow)
	if !goDAGContextHasEdge(spec, "implementation_context_fanin", "execution") {
		t.Fatalf("workflow edges = %#v, want implementation_context_fanin -> execution", spec.Edges)
	}
}

// TestWorkflowSpecSeparatesSubagentDisplayAndRunStages keeps graph ownership distinct from scheduling.
func TestWorkflowSpecSeparatesSubagentDisplayAndRunStages(t *testing.T) {
	workflow := DefaultWorkflowConfig()
	workflow.Parallel.Groups["planning_context"] = ParallelGroupConfig{
		Mode: "advisory",
		Members: []ParallelMemberConfig{
			{Name: "需求分析员", Purpose: "找出需求歧义、风险和遗漏", Stage: "planning", Tool: "pi"},
		},
	}
	spec := BuildWorkflowSpec("demo", workflow)

	planning := goDAGContextNodeByID(t, spec, "planning_context_1")
	if planning.Stage != "planning" || planning.RunStage != "execution" {
		t.Fatalf("planning node stage/run_stage = %q/%q, want planning/execution", planning.Stage, planning.RunStage)
	}
	implementation := goDAGContextNodeByID(t, spec, "implementation_context_1")
	if implementation.Stage != "execution" || implementation.RunStage != "execution" {
		t.Fatalf("implementation node stage/run_stage = %q/%q, want execution/execution", implementation.Stage, implementation.RunStage)
	}
}

// TestDefaultWorkflowSpecOmitsPlanningContext keeps sealed runs from repeating planning helpers by default.
func TestDefaultWorkflowSpecOmitsPlanningContext(t *testing.T) {
	spec := BuildWorkflowSpec("demo", DefaultWorkflowConfig())
	for _, node := range spec.Nodes {
		if node.Group == "planning_context" || node.ID == "planning_context_1" {
			t.Fatalf("default workflow must not schedule planning_context node: %#v", node)
		}
	}
}

// TestGoDAGRunsExecutionContextWhenTasksPending verifies pending changes still run execution helpers.
func TestGoDAGRunsExecutionContextWhenTasksPending(t *testing.T) {
	repo := goDAGContextRepo(t)
	goDAGContextChange(t, repo, "- [ ] task\n")
	goDAGContextInstallFakeOz(t)
	runID := "pending-execution-context-run"
	if err := snapshotRunPrompts(repo, runID); err != nil {
		t.Fatal(err)
	}
	state := goDAGContextState(t, repo, runID)
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	runner := &goDAGContextFakeRunner{}
	engine := NewEngine(repo, goDAGContextRegistry(runner))

	for _, node := range []WorkflowNode{
		{ID: "implementation_context_1", Type: "subagent", Group: "before_execution", Stage: "execution", Member: "代码库侦察员"},
		{ID: "implementation_context_2", Type: "subagent", Group: "before_execution", Stage: "execution", Member: "外部资料研究员"},
		{ID: "before_execution_fanin", Type: "fanin", Group: "before_execution", Stage: "execution"},
		{ID: "execution", Type: "main_stage", Stage: "execution"},
	} {
		if err := engine.runGoDAGNode(context.Background(), runID, node); err != nil {
			t.Fatal(err)
		}
	}

	persisted, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if runner.mainCalls != 1 {
		t.Fatalf("main calls = %d, want one execution call", runner.mainCalls)
	}
	if persisted.Stage != "archive" || persisted.Stages["execution"] != "completed" {
		t.Fatalf("state = stage %q execution %q, want archive/completed", persisted.Stage, persisted.Stages["execution"])
	}
	if len(persisted.Sessions) != 3 {
		t.Fatalf("sessions = %#v, want two subagents and executor", persisted.Sessions)
	}
	if !fileExists(parallelArtifactPath(runDir(repo, runID), "implementation_context", 0)) {
		t.Fatal("implementation context fan-in artifact should exist")
	}
}

// goDAGContextHasEdge reports whether the workflow graph contains a concrete dependency edge.
func goDAGContextHasEdge(spec WorkflowSpec, from string, to string) bool {
	for _, edge := range spec.Edges {
		if edge.From == from && edge.To == to {
			return true
		}
	}
	return false
}

// goDAGContextNodeByID returns a graph node by stable ID for graph-shape tests.
func goDAGContextNodeByID(t *testing.T, spec WorkflowSpec, id string) WorkflowNode {
	t.Helper()
	for _, node := range spec.Nodes {
		if node.ID == id {
			return node
		}
	}
	t.Fatalf("node %q not found in %#v", id, spec.Nodes)
	return WorkflowNode{}
}

// goDAGContextState builds a running execution state with implementation context enabled.
func goDAGContextState(t *testing.T, repo string, runID string) State {
	t.Helper()
	head, diff, err := gitSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	workflow := DefaultWorkflowConfig()
	workflow.Engine = "go-dag"
	workflow.MaxReviewIterations = 0
	workflow.Parallel = ParallelConfig{
		Enabled: true,
		Groups: map[string]ParallelGroupConfig{
			"implementation_context": {
				Mode: "advisory",
				Members: []ParallelMemberConfig{
					{Name: "代码库侦察员", Purpose: "汇总 execution 需要读取的文件和测试模式", Stage: "before_execution", Tool: "codex"},
					{Name: "外部资料研究员", Purpose: "查询 execution 依赖的外部库文档", Stage: "before_execution", Tool: "codex"},
				},
			},
		},
	}
	workflow.Validation = ValidationConfig{MaxAttemptsPerStage: 3}
	return State{
		RunID:        runID,
		ChangeName:   "demo",
		Status:       statusRunning,
		Stage:        "execution",
		BaselineHead: head,
		BaselineDiff: diff,
		Sessions:     map[string]string{},
		Stages:       map[string]string{},
		Workflow:     workflow,
	}
}

// goDAGContextRegistry maps supported agent backends to the deterministic fake runner.
func goDAGContextRegistry(runner AgentRunner) *AgentRegistry {
	registry := &AgentRegistry{}
	tool := goDAGContextFakeTool{runner: runner}
	registry.Register(tool)
	registry.Register(goDAGContextNamedTool{name: "pi", runner: runner})
	registry.Register(goDAGContextNamedTool{name: "agy", runner: runner})
	return registry
}

// goDAGContextNamedTool gives the same fake runner multiple configured tool names.
type goDAGContextNamedTool struct {
	name   string
	runner AgentRunner
}

func (t goDAGContextNamedTool) Name() string { return t.name }

func (goDAGContextNamedTool) Resolve() error { return nil }

func (goDAGContextNamedTool) PlanningCommand(context.Context, string, string, io.Reader, StageOptions) (*exec.Cmd, error) {
	return nil, nil
}

func (t goDAGContextNamedTool) NewRunner() AgentRunner { return t.runner }

// goDAGContextRepo creates a committed git repo for manual-intervention guards.
func goDAGContextRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGitForGoDAGContext(t, repo, "init")
	runGitForGoDAGContext(t, repo, "config", "user.email", "test@example.com")
	runGitForGoDAGContext(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitForGoDAGContext(t, repo, "add", ".")
	runGitForGoDAGContext(t, repo, "commit", "-m", "init")
	return repo
}

// goDAGContextChange writes a minimal active oz change with caller-controlled task state.
func goDAGContextChange(t *testing.T, repo, task string) {
	t.Helper()
	root := filepath.Join(repo, "docs", "changes", "demo")
	if err := os.MkdirAll(filepath.Join(root, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"brief.md":        "demo\n",
		"proposal.md":     "demo\n",
		"design.md":       "demo\n",
		"spec.md":         "demo\n",
		"task.md":         task,
		"acceptance.json": `{"summary":"demo","coverage":[{"spec":"demo workflow","tests":["contract-demo"],"evidence":["runtime-demo"],"risk":"covered by runtime log"}],"required_tests":[{"id":"contract-demo","source":"change_contract","path":"docs/changes/demo/tests/demo.acceptance.test.ts","command":"pnpm exec tsx --test docs/changes/demo/tests/demo.acceptance.test.ts","purpose":"produce runtime-demo at test-results/demo.log","assertions":["execution writes runtime-demo to test-results/demo.log"]}],"required_evidence":[{"id":"runtime-demo","kind":"runtime_log","path":"test-results/demo.log","purpose":"prove demo runtime path"}]}` + "\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// goDAGContextInstallFakeOz points ChangeTasksDone at the test-process oz fixture.
func goDAGContextInstallFakeOz(t *testing.T) {
	t.Helper()
	previous := ozCommand
	previousPrefix := ozCommandPrefix
	ozCommand = os.Args[0]
	ozCommandPrefix = []string{"-test.run=TestGoDAGContextFakeOzCommand", "--"}
	t.Setenv("WO_GO_DAG_CONTEXT_FAKE_OZ", "1")
	t.Cleanup(func() {
		ozCommand = previous
		ozCommandPrefix = previousPrefix
	})
}

// TestGoDAGContextFakeOzCommand serves oz status JSON in a child test process.
func TestGoDAGContextFakeOzCommand(t *testing.T) {
	if os.Getenv("WO_GO_DAG_CONTEXT_FAKE_OZ") != "1" {
		return
	}
	args := os.Args
	for i, arg := range args {
		if arg == "--" {
			goDAGContextFakeOzMain(args[i+1:])
			os.Exit(0)
		}
	}
	os.Exit(1)
}

// goDAGContextFakeOzMain implements the minimal oz status command used by these tests.
func goDAGContextFakeOzMain(args []string) {
	if len(args) < 2 || args[0] != "status" {
		os.Exit(1)
	}
	data, err := os.ReadFile(filepath.Join("docs", "changes", args[1], "task.md"))
	if err != nil {
		os.Exit(1)
	}
	total := strings.Count(string(data), "- [ ]") + strings.Count(string(data), "- [x]") + strings.Count(string(data), "- [X]")
	done := strings.Count(string(data), "- [x]") + strings.Count(string(data), "- [X]")
	_, _ = os.Stdout.WriteString(`{"tasks":{"total":` + strconv.Itoa(total) + `,"done":` + strconv.Itoa(done) + `}}` + "\n")
	os.Exit(0)
}

// runGitForGoDAGContext runs git commands in the temporary test repository.
func runGitForGoDAGContext(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
