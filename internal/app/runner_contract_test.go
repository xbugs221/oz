// Package app tests the wo runner JSON contract through realistic CLI flows.
package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunVersionAndContractExposeCapabilities verifies discovery works without agent backends.
func TestRunVersionAndContractExposeCapabilities(t *testing.T) {
	repo := gitRepo(t)
	chdir(t, repo)
	oldVersion := Version
	Version = "v9.8.7"
	t.Cleanup(func() { Version = oldVersion })
	cases := [][]string{{"--version"}, {"contract", "--json"}}
	for _, args := range cases {
		var stdout, stderr bytes.Buffer
		if err := Run(args, strings.NewReader(""), &stdout, &stderr); err != nil {
			t.Fatalf("Run(%v) error = %v, stderr = %q", args, err, stderr.String())
		}
		if strings.TrimSpace(stdout.String()) == "" {
			t.Fatalf("Run(%v) produced empty stdout", args)
		}
	}
	var versionOut bytes.Buffer
	if err := Run([]string{"--version"}, strings.NewReader(""), &versionOut, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(versionOut.String()); got != "v9.8.7" {
		t.Fatalf("version = %q, want git tag v9.8.7", got)
	}

	var stdout bytes.Buffer
	if err := Run([]string{"contract", "--json"}, strings.NewReader(""), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var contract runnerContract
	if err := json.Unmarshal(stdout.Bytes(), &contract); err != nil {
		t.Fatal(err)
	}
	if contract.Version != "v9.8.7" {
		t.Fatalf("contract version = %q, want git tag v9.8.7", contract.Version)
	}
	for _, capability := range runnerCapabilities {
		if !containsString(contract.Capabilities, capability) {
			t.Fatalf("capabilities = %v, missing %s", contract.Capabilities, capability)
		}
	}
}

// TestRunVersionReportsGitTagDescriptionFromSourceRepo verifies local source builds use git tags.
func TestRunVersionReportsGitTagDescriptionFromSourceRepo(t *testing.T) {
	oldVersion := Version
	Version = "dev"
	t.Cleanup(func() { Version = oldVersion })
	wantBytes, err := exec.Command("git", "describe", "--tags", "--always", "--dirty").Output()
	if err != nil {
		t.Skipf("git tag description unavailable: %v", err)
	}
	nonGitDir := t.TempDir()
	chdir(t, nonGitDir)
	var stdout, stderr bytes.Buffer
	if err := Run([]string{"--version"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("version failed outside git repo: %v, stderr = %q", err, stderr.String())
	}
	got := strings.TrimSpace(stdout.String())
	want := strings.TrimSpace(string(wantBytes))
	if got == "dev" || got != want {
		t.Fatalf("version should use git tag description, got %q want %q", got, want)
	}
}

// TestRunHelpDocumentsHumanAndRunnerCommands verifies help is useful at the CLI.
func TestRunHelpDocumentsHumanAndRunnerCommands(t *testing.T) {
	repo := gitRepo(t)
	chdir(t, repo)
	var stdout bytes.Buffer
	if err := Run([]string{"--help"}, strings.NewReader(""), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	help := stdout.String()
	for _, want := range []string{
		"用法：",
		"wo config [--global]",
		"wo status",
		"wo update",
		"wo --run <change-name>",
		"Runner JSON 命令：",
		"wo run --change <change-name> --json",
		"wo status --run-id <run-id> --json",
		"wo.yaml",
		"${XDG_STATE_HOME:-~/.local/state}/wo/repos/<repo-key>/runs/<run-id>/",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q:\n%s", want, help)
		}
	}
}

// TestRunHumanStatusPrintsNewestChecklist verifies README's `wo status` command stays human-readable.
func TestRunHumanStatusPrintsNewestChecklist(t *testing.T) {
	repo := gitRepo(t)
	chdir(t, repo)
	state := State{
		RunID:      "demo-run",
		ChangeName: "demo",
		Status:     statusRunning,
		Stage:      "review_1",
		Sessions:   map[string]string{"codex:executor": "executor-thread", "codex:reviewer": "reviewer-thread"},
		Stages:     map[string]string{"execution": "completed"},
		Workflow:   DefaultWorkflowConfig(),
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := Run([]string{"status"}, strings.NewReader(""), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	for _, want := range []string{"- 写 executor-thread ✓", "- 审 reviewer-thread →"} {
		if !strings.Contains(got, want) {
			t.Fatalf("status missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "{") || strings.Contains(got, "pid=") || strings.Contains(got, "thread=") {
		t.Fatalf("status should be compact human checklist:\n%s", got)
	}
}

// TestRunHumanStatusMarksBlockedReviewLimit verifies terminal review blocks are visible.
func TestRunHumanStatusMarksBlockedReviewLimit(t *testing.T) {
	repo := gitRepo(t)
	chdir(t, repo)
	workflow := zeroReviewWorkflow()
	workflow.MaxReviewIterations = 1
	workflow.Stages["review_1"] = StageOptions{Tool: "codex", Reasoning: "high"}
	workflow.Stages["fix_1"] = StageOptions{Tool: "codex", Reasoning: "low"}
	state := State{
		RunID:      "blocked-run",
		ChangeName: "demo",
		Status:     statusBlocked,
		Stage:      statusBlocked,
		Error:      "审核修正达到上限，工作流已中断",
		Sessions:   map[string]string{"codex:executor": "executor-thread", "codex:reviewer": "reviewer-thread"},
		Stages:     map[string]string{"execution": "completed", "review_1": "completed", "fix_1": "completed"},
		Workflow:   workflow,
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := Run([]string{"status"}, strings.NewReader(""), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	for _, want := range []string{"blocked_review_limit", " x ", "审核修正达到上限"} {
		if !strings.Contains(got, want) {
			t.Fatalf("status missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "→") {
		t.Fatalf("blocked status must not look running:\n%s", got)
	}

	stdout.Reset()
	if err := Run([]string{"status", "--run-id", "blocked-run", "--json"}, strings.NewReader(""), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var dto RunnerState
	if err := json.Unmarshal(stdout.Bytes(), &dto); err != nil {
		t.Fatalf("json status = %q: %v", stdout.String(), err)
	}
	if dto.Status != statusBlocked || dto.Stage != statusBlocked {
		t.Fatalf("json status = %#v, want blocked_review_limit contract", dto)
	}
}

// TestRunHumanStatusUsesNewestRun verifies stale unfinished runs do not hide a newer run.
func TestRunHumanStatusUsesNewestRun(t *testing.T) {
	repo := gitRepo(t)
	chdir(t, repo)
	old := State{
		RunID:      "20260508T163350.431416617Z",
		ChangeName: "demo",
		Status:     statusRunning,
		Stage:      "review_1",
		Sessions:   map[string]string{"codex:executor": "old-executor"},
		Stages:     map[string]string{"execution": "completed"},
		Workflow:   DefaultWorkflowConfig(),
	}
	newer := State{
		RunID:      "20260508T163733.263967687Z",
		ChangeName: "demo",
		Status:     statusDone,
		Stage:      "done",
		Sessions:   map[string]string{"codex:executor": "new-executor", "codex:reviewer": "new-reviewer", "codex:archiver": "new-archiver"},
		Stages:     map[string]string{"execution": "completed", "review_1": "completed", "archive": "completed"},
		Workflow:   DefaultWorkflowConfig(),
	}
	if err := saveState(repo, old); err != nil {
		t.Fatal(err)
	}
	if err := saveState(repo, newer); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := Run([]string{"status"}, strings.NewReader(""), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	if strings.Contains(got, "old-executor") || strings.Contains(got, "- 审 new-reviewer →") {
		t.Fatalf("status used stale unfinished run:\n%s", got)
	}
	for _, want := range []string{"- 写 new-executor ✓", "- 审 new-reviewer ✓", "- 存 new-archiver ✓"} {
		if !strings.Contains(got, want) {
			t.Fatalf("status missing newest run line %q:\n%s", want, got)
		}
	}
}

// TestRunInstallReturnsMigrationError verifies old prompt install is rejected.
func TestRunInstallCopiesPromptTemplates(t *testing.T) {
	repo := gitRepo(t)
	chdir(t, repo)

	err := Run([]string{"install"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "prompt 已内嵌") {
		t.Fatalf("install error = %v, want YAML migration error", err)
	}
}

// TestRunConfigWritesDefaultConfig verifies the CLI creates wo.yaml in the repo root.
func TestRunInitWritesDefaultConfig(t *testing.T) {
	repo := gitRepo(t)
	chdir(t, repo)
	var stdout bytes.Buffer
	if err := Run([]string{"config"}, strings.NewReader(""), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stdout.String()) != "已创建 wo.yaml" {
		t.Fatalf("stdout = %q, want 已创建 wo.yaml", stdout.String())
	}
	if _, err := LoadWorkflowConfig(repo); err != nil {
		t.Fatal(err)
	}
	if fileExists(filepath.Join(repo, ".wo")) {
		t.Fatal("config must not create .wo")
	}
	if err := Run([]string{"config"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected existing config error")
	}
	err := Run([]string{"init"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "wo config") {
		t.Fatalf("init error = %v, want migration error", err)
	}
}

// TestRunConfigGlobalDoesNotRequireGitRepo verifies user defaults can be created anywhere.
func TestRunConfigGlobalDoesNotRequireGitRepo(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, dir)

	var stdout bytes.Buffer
	if err := Run([]string{"config", "--global"}, strings.NewReader(""), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(home, "wo.yaml")) {
		t.Fatal("wo config --global did not create ~/wo.yaml")
	}
	if fileExists(filepath.Join(home, ".wo")) {
		t.Fatal("wo config --global must not create ~/.wo")
	}
}

// TestRunListChangesJSONUsesOzList verifies JSON listing mirrors oz list output.
func TestRunListChangesJSONUsesOzList(t *testing.T) {
	repo := gitRepo(t)
	chdir(t, repo)
	mustChange(t, repo, "demo-change")
	mustChange(t, repo, "archive")
	mustChange(t, repo, ".hidden")
	installFakeOz(t, "demo-change")
	if err := os.MkdirAll(filepath.Join(repo, "docs", "changes", "invalid"), 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := Run([]string{"list-changes", "--json"}, strings.NewReader(""), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var got changeList
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Changes) != 1 || got.Changes[0].Name != "demo-change" {
		t.Fatalf("changes = %#v, want only demo-change", got.Changes)
	}
}

// TestChangeListTitleKeepsChineseNameReadable verifies JSON titles do not corrupt UTF-8 names.
func TestChangeListTitleKeepsChineseNameReadable(t *testing.T) {
	repo := gitRepo(t)
	chdir(t, repo)
	mustChange(t, repo, "1-适配-oz-并重设计终端输出")

	var stdout bytes.Buffer
	if err := Run([]string{"list-changes", "--json"}, strings.NewReader(""), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var got changeList
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Changes) != 1 {
		t.Fatalf("changes = %#v, want one change", got.Changes)
	}
	if got.Changes[0].Title != "1 适配 Oz 并重设计终端输出" {
		t.Fatalf("title = %q", got.Changes[0].Title)
	}
}

// TestEngineStartJSONEmitsStateBeforeContinuing verifies run JSON starts with a durable state id.
func TestEngineStartJSONEmitsStateBeforeContinuing(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  workflow:\n    max_review_iterations: 0\n")
	engine := NewEngine(repo, testRegistry(fakeRunner{}))
	var stdout bytes.Buffer
	if err := engine.StartJSON(context.Background(), "demo", &stdout); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "{") {
		t.Fatalf("stdout = %q, want one JSON line", stdout.String())
	}
	var start RunnerState
	if err := json.Unmarshal([]byte(lines[0]), &start); err != nil {
		t.Fatal(err)
	}
	if start.RunID == "" || start.Status != statusRunning || start.Stage != "execution" {
		t.Fatalf("start DTO = %#v", start)
	}
	if !fileExists(filepath.Join(runDir(repo, start.RunID), "state.json")) {
		t.Fatalf("state.json for %s was not created before start DTO", start.RunID)
	}
	final, err := loadState(repo, start.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != statusDone {
		t.Fatalf("final status = %s, want done", final.Status)
	}
}

// TestEngineStartJSONPersistsAgentProgressWithoutStdout verifies JSON mode keeps state fresh.
func TestEngineStartJSONPersistsAgentProgressWithoutStdout(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  workflow:\n    max_review_iterations: 0\n")
	runner := &progressCallbackRunner{}
	engine := NewEngine(repo, testRegistry(runner))
	engine.Output = nil
	var stdout bytes.Buffer
	if err := engine.StartJSON(context.Background(), "demo", &stdout); err != nil {
		t.Fatal(err)
	}
	if runner.progress == nil {
		t.Fatal("json mode should install a state progress writer")
	}
	if !runner.sawRunningStage || !runner.sawProgressSession {
		t.Fatalf("state was not updated from progress: running=%v session=%v", runner.sawRunningStage, runner.sawProgressSession)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("stdout = %q, want exactly one JSON line", stdout.String())
	}
}

// TestRunStatusAndAbortJSONUseSpecificRun verifies script commands target the requested run id.
func TestRunStatusAndAbortJSONUseSpecificRun(t *testing.T) {
	repo := gitRepo(t)
	chdir(t, repo)
	state := State{RunID: "demo-run", ChangeName: "demo", Status: statusRunning, Stage: "review_1"}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir(repo, "demo-run"), "lock"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var statusOut bytes.Buffer
	if err := Run([]string{"status", "--run-id", "demo-run", "--json"}, strings.NewReader(""), &statusOut, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var status RunnerState
	if err := json.Unmarshal(statusOut.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(statusOut.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["runId"]; ok {
		t.Fatalf("status JSON should not include runId: %s", statusOut.String())
	}
	if _, ok := raw["changeName"]; ok {
		t.Fatalf("status JSON should not include changeName: %s", statusOut.String())
	}
	if status.RunID != "demo-run" || status.Status != statusRunning || status.Stage != "review_1" || status.Paths == nil || status.Sessions == nil || status.Stages == nil {
		t.Fatalf("status DTO = %#v", status)
	}

	var abortOut bytes.Buffer
	if err := Run([]string{"abort", "--run-id", "demo-run", "--json"}, strings.NewReader(""), &abortOut, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var aborted RunnerState
	if err := json.Unmarshal(abortOut.Bytes(), &aborted); err != nil {
		t.Fatal(err)
	}
	if aborted.RunID != "demo-run" || aborted.Status != "aborted" || aborted.Stage != "review_1" {
		t.Fatalf("abort DTO = %#v", aborted)
	}
	if fileExists(filepath.Join(runDir(repo, "demo-run"), "lock")) {
		t.Fatal("abort left lock file behind")
	}
}

type progressCallbackRunner struct {
	progress           io.Writer
	sawRunningStage    bool
	sawProgressSession bool
}

// SetProgress records the human progress writer installed by the engine.
func (r *progressCallbackRunner) SetProgress(progress io.Writer) {
	r.progress = progress
}

// Run writes progress when installed and then delegates artifact creation to fakeRunner.
func (r *progressCallbackRunner) Run(ctx context.Context, repo, prompt, threadID string, options StageOptions) (string, error) {
	if r.progress != nil {
		_, _ = r.progress.Write([]byte("agent process started: pid=123\n"))
		_, _ = r.progress.Write([]byte("agent session started: session=fake-thread\n"))
		if state, err := loadState(repo, currentRunID(repo)); err == nil {
			if state.Stage == "execution" {
				r.sawRunningStage = state.Stages["execution"] == "running"
				r.sawProgressSession = state.Sessions["codex:executor"] == "fake-thread"
			}
		}
	}
	return fakeRunner{}.Run(ctx, repo, prompt, threadID, options)
}

// TestEngineResumeRunJSONUsesRequestedRun verifies resume does not choose the newest run implicitly.
func TestEngineResumeRunJSONUsesRequestedRun(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	head, diff, err := gitSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	old := State{RunID: "old-run", ChangeName: "demo", Sealed: true, Status: statusRunning, Stage: "execution", BaselineHead: head, BaselineDiff: diff, Workflow: zeroReviewWorkflow()}
	newer := State{RunID: "newer-run", ChangeName: "demo", Sealed: true, Status: statusRunning, Stage: "review_1", BaselineHead: head, BaselineDiff: diff, Workflow: zeroReviewWorkflow()}
	if err := saveState(repo, old); err != nil {
		t.Fatal(err)
	}
	if err := saveState(repo, newer); err != nil {
		t.Fatal(err)
	}
	for _, runID := range []string{"old-run", "newer-run"} {
		if err := snapshotRunPrompts(repo, runID); err != nil {
			t.Fatal(err)
		}
	}
	engine := NewEngine(repo, testRegistry(fakeRunner{}))
	var stdout bytes.Buffer
	if err := engine.ResumeRunJSON(context.Background(), "old-run", &stdout); err != nil {
		t.Fatal(err)
	}
	var start RunnerState
	if err := json.Unmarshal([]byte(strings.Split(strings.TrimSpace(stdout.String()), "\n")[0]), &start); err != nil {
		t.Fatal(err)
	}
	if start.RunID != "old-run" {
		t.Fatalf("resume DTO run_id = %s, want old-run", start.RunID)
	}
	finalOld, err := loadState(repo, "old-run")
	if err != nil {
		t.Fatal(err)
	}
	finalNewer, err := loadState(repo, "newer-run")
	if err != nil {
		t.Fatal(err)
	}
	if finalOld.Status != statusDone || finalNewer.Status != statusRunning {
		t.Fatalf("old/newer status = %s/%s, want done/running", finalOld.Status, finalNewer.Status)
	}
}

// TestRunResumeJSONDoesNotPersistFailureForActiveLock verifies live workers are not overwritten.
func TestRunResumeJSONDoesNotPersistFailureForActiveLock(t *testing.T) {
	repo := gitRepo(t)
	chdir(t, repo)
	mustChange(t, repo, "demo")
	runID := "locked-run"
	state := State{
		RunID:      runID,
		ChangeName: "demo",
		Status:     statusRunning,
		Stage:      "execution",
		Sessions:   map[string]string{},
		Stages:     map[string]string{"execution": "running"},
		Paths:      map[string]string{},
		Workflow:   DefaultWorkflowConfig(),
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	hostname, _ := os.Hostname()
	mustWriteLock(t, repo, runID, LockInfo{PID: os.Getpid(), Hostname: hostname, RunID: runID})

	var stdout bytes.Buffer
	err := Run([]string{"resume", "--run-id", runID, "--json"}, strings.NewReader(""), &stdout, &bytes.Buffer{})
	if err == nil || !isRunLockedError(err) {
		t.Fatalf("resume error = %v, want run lock conflict", err)
	}
	var dto RunnerState
	if err := json.Unmarshal(stdout.Bytes(), &dto); err != nil {
		t.Fatalf("stdout = %q: %v", stdout.String(), err)
	}
	if dto.Status != statusRunning || dto.Error == "" {
		t.Fatalf("resume DTO = %#v, want running state with lock error", dto)
	}
	final, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != statusRunning || final.Error != "" {
		t.Fatalf("durable state = %#v, want unchanged running state", final)
	}
}

// TestRunJSONErrorsCoverRequiredFailureModes verifies 6.8 runner error behavior.
func TestRunJSONErrorsCoverRequiredFailureModes(t *testing.T) {
	repo := gitRepo(t)
	chdir(t, repo)
	mustChange(t, repo, "demo")
	if err := os.MkdirAll(runDir(repo, "bad-run"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir(repo, "bad-run"), "state.json"), []byte("{bad json}"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name       string
		args       []string
		wantStdout bool
		wantRunID  string
	}{
		{name: "missing run id", args: []string{"status", "--json"}},
		{name: "unknown status run", args: []string{"status", "--run-id", "missing", "--json"}, wantStdout: true, wantRunID: "missing"},
		{name: "unknown resume run", args: []string{"resume", "--run-id", "missing", "--json"}, wantStdout: true, wantRunID: "missing"},
		{name: "unknown abort run", args: []string{"abort", "--run-id", "missing", "--json"}, wantStdout: true, wantRunID: "missing"},
		{name: "unknown change", args: []string{"run", "--change", "missing", "--json"}},
		{name: "bad state", args: []string{"status", "--run-id", "bad-run", "--json"}, wantStdout: true, wantRunID: "bad-run"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			err := Run(tc.args, strings.NewReader(""), &stdout, &bytes.Buffer{})
			if err == nil {
				t.Fatalf("Run(%v) succeeded, want error", tc.args)
			}
			if !tc.wantStdout {
				if stdout.Len() != 0 {
					t.Fatalf("stdout = %q, want empty", stdout.String())
				}
				return
			}
			var failed RunnerState
			if err := json.Unmarshal(stdout.Bytes(), &failed); err != nil {
				t.Fatalf("stdout = %q is not failed DTO: %v", stdout.String(), err)
			}
			if failed.RunID != tc.wantRunID || failed.Status != "failed" || failed.Stage != "" || failed.Error == "" {
				t.Fatalf("failed DTO = %#v", failed)
			}
		})
	}
}

// TestEngineStartReportsMissingConfiguredAgentTool verifies bad workflow backends fail before run state is created.
func TestEngineStartReportsMissingConfiguredAgentTool(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), `
wo:
  workflow:
    max_review_iterations: 0
    stages:
      execution:
        tool: opencode
`)
	registry := &AgentRegistry{}
	registry.Register(fakeTool{name: "codex", runner: fakeRunner{}})
	engine := NewEngine(repo, registry)
	err := engine.Start(context.Background(), "demo")
	if err == nil || !strings.Contains(err.Error(), `未知 agent tool "opencode"`) {
		t.Fatalf("Start error = %v, want missing opencode tool", err)
	}
	if fileExists(filepath.Join(repo, ".wo", "runs")) {
		t.Fatal("missing tool should fail before creating sealed run state")
	}
}

// TestBackendFailureProgressAndJSONState verifies runner failures are both visible and machine-readable.
func TestBackendFailureProgressAndJSONState(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  workflow:\n    max_review_iterations: 0\n")
	runner := &failingProgressRunner{}
	engine := NewEngine(repo, testRegistry(runner))
	var progress bytes.Buffer
	engine.Output = &progress

	var stdout bytes.Buffer
	err := engine.StartJSON(context.Background(), "demo", &stdout)
	if err == nil || !strings.Contains(err.Error(), "agent crashed") {
		t.Fatalf("StartJSON error = %v, want agent crash", err)
	}
	if !strings.Contains(progress.String(), "失败") {
		t.Fatalf("progress = %q, want visible failed stage", progress.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdout = %q, want start and failed DTOs", stdout.String())
	}
	var start RunnerState
	if err := json.Unmarshal([]byte(lines[0]), &start); err != nil {
		t.Fatalf("start DTO = %q: %v", lines[0], err)
	}
	var failed RunnerState
	if err := json.Unmarshal([]byte(lines[1]), &failed); err != nil {
		t.Fatalf("failed DTO = %q: %v", lines[1], err)
	}
	if failed.RunID != start.RunID || failed.Status != "failed" || !strings.Contains(failed.Error, "agent crashed") {
		t.Fatalf("failed DTO = %#v, start = %#v", failed, start)
	}
	state, err := loadState(repo, start.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != "failed" || !strings.Contains(state.Error, "agent crashed") {
		t.Fatalf("state = %#v, want persisted failed run", state)
	}
}

// TestRunJSONBackendFailureWritesFailedDTO verifies the public runner command does not leave callers at running.
func TestRunJSONBackendFailureWritesFailedDTO(t *testing.T) {
	repo := gitRepo(t)
	chdir(t, repo)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  workflow:\n    max_review_iterations: 0\n    parallel:\n      enabled: false\n")
	installFakeOz(t, "demo")
	installFailingCodex(t)

	var stdout bytes.Buffer
	err := Run([]string{"run", "--change", "demo", "--json"}, strings.NewReader(""), &stdout, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "codex failed for test") {
		t.Fatalf("Run error = %v, want codex failure", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdout = %q, want start and failed DTOs", stdout.String())
	}
	var failed RunnerState
	if err := json.Unmarshal([]byte(lines[1]), &failed); err != nil {
		t.Fatalf("failed DTO = %q: %v", lines[1], err)
	}
	if failed.Status != "failed" || failed.Error == "" {
		t.Fatalf("failed DTO = %#v", failed)
	}
}

type failingProgressRunner struct {
	progress io.Writer
}

// SetProgress captures backend progress so tests assert user-visible failure events.
func (r *failingProgressRunner) SetProgress(progress io.Writer) {
	r.progress = progress
}

// Run emits the generic failed-session line and returns a deterministic backend error.
func (r *failingProgressRunner) Run(context.Context, string, string, string, StageOptions) (string, error) {
	printAgentSessionFailed(r.progress, "codex")
	return "", fmt.Errorf("agent crashed")
}

// installFailingCodex puts a deterministic codex shim on PATH for public CLI tests.
func installFailingCodex(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "codex")
	body := "#!/usr/bin/env bash\ncat >/dev/null\nprintf '{\"type\":\"thread.started\",\"thread_id\":\"fake-thread\"}\\n'\nprintf 'codex failed for test\\n' >&2\nexit 9\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// zeroReviewWorkflow returns a fast workflow that can complete without review artifacts.
func zeroReviewWorkflow() WorkflowConfig {
	workflow := DefaultWorkflowConfig()
	workflow.Engine = "legacy"
	workflow.MaxReviewIterations = 0
	workflow.Parallel.Enabled = false
	return workflow
}

// containsString reports whether a string slice includes a value.
func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// chdir runs CLI tests from the target repository root.
func chdir(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatal(err)
		}
	})
}
