// Package app verifies migrated shell contracts through long-lived Go tests.
package app

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestBundledOzSkillPromptsDelegateToSkills verifies agent prompts do not duplicate oz skill bodies.
func TestBundledOzSkillPromptsDelegateToSkills(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mustHave   []string
		mustReject []string
	}{
		{name: "oz-flow-discuss", mustHave: []string{"oz-plan", "讨论规划阶段"}, mustReject: []string{"change-name", "open questions"}},
		{name: "oz-flow-start", mustHave: []string{"oz-exec", "state.json.change_name", "当前 oz change", "acceptance.json", "不要超出当前提案范围"}, mustReject: []string{"proposal.md", "design.md", "spec.md", "required_tests", "oz status", "tasks.done", "review-1.json", "fix-1-summary.md", "只修复当前 review/QA artifact 中列出的 findings"}},
		{name: "oz-flow-done", mustHave: []string{"oz-archive", "delivery-summary.md", "最终审核"}, mustReject: []string{"oz status", "oz validate", "oz archive", "--yes", "tasks.total", "tasks.done", "delta specs", "git commit"}},
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

// TestDetectManualInterventionAllowsUnrelatedActiveChange verifies users can record new demand while a run is active.
func TestDetectManualInterventionAllowsUnrelatedActiveChange(t *testing.T) {
	repo := gitRepoForMigratedContract(t)
	mustChangeForMigratedContract(t, repo, "10-当前需求")
	runGitForMigratedContract(t, repo, "add", ".")
	runGitForMigratedContract(t, repo, "commit", "-m", "current change baseline")
	head, diff, err := gitSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	state := migratedContractState("running-demand-insertion", "10-当前需求", "execution", head, diff)
	mustWriteForMigratedContract(t, filepath.Join(repo, "docs", "changes", "11-运行中新需求", "brief.md"), "# 新需求\n")
	engine := NewEngine(repo, migratedContractRegistry())
	if err := engine.detectManualIntervention(&state); err != nil {
		t.Fatalf("unrelated active change should not abort current run: %v", err)
	}
	if state.Status == statusAborted {
		t.Fatal("unrelated active change aborted current run")
	}
	if !strings.Contains(state.BaselineDiff, "11-运行中新需求") {
		t.Fatalf("baseline diff did not absorb new demand:\n%s", state.BaselineDiff)
	}
}

// TestDetectManualInterventionIgnoresExistingProtectedBaselineDiff verifies only new delta paths are guarded.
func TestDetectManualInterventionIgnoresExistingProtectedBaselineDiff(t *testing.T) {
	repo := gitRepoForMigratedContract(t)
	mustChangeForMigratedContract(t, repo, "10-当前需求")
	mustWriteForMigratedContract(t, filepath.Join(repo, "internal", "app", "existing.go"), "package app\n")
	runGitForMigratedContract(t, repo, "add", ".")
	runGitForMigratedContract(t, repo, "commit", "-m", "current change baseline")
	mustWriteForMigratedContract(t, filepath.Join(repo, "docs", "changes", "10-当前需求", "task.md"), "- [x] execution edit\n")
	mustWriteForMigratedContract(t, filepath.Join(repo, "internal", "app", "existing.go"), "package app\n\nconst executionEdit = true\n")
	head, diff, err := gitSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "10-当前需求") || !strings.Contains(diff, "existing.go") {
		t.Fatalf("test baseline did not include protected dirty paths:\n%s", diff)
	}
	state := migratedContractState("running-demand-existing-baseline", "10-当前需求", "review_1", head, diff)
	state.Stages = map[string]string{"execution": "completed"}
	mustWriteForMigratedContract(t, filepath.Join(repo, "docs", "changes", "11-运行中新需求", "brief.md"), "# 新需求\n")
	engine := NewEngine(repo, migratedContractRegistry())
	if err := engine.detectManualIntervention(&state); err != nil {
		t.Fatalf("unrelated demand should compare against baseline delta only: %v", err)
	}
	if !strings.Contains(state.BaselineDiff, "11-运行中新需求") {
		t.Fatalf("baseline diff did not absorb unrelated demand:\n%s", state.BaselineDiff)
	}
}

// TestDetectManualInterventionBlocksCurrentRunPaths verifies the narrowed guard still blocks protected writes.
func TestDetectManualInterventionBlocksCurrentRunPaths(t *testing.T) {
	cases := []struct {
		name  string
		write func(t *testing.T, repo string)
	}{
		{name: "source", write: func(t *testing.T, repo string) {
			mustWriteForMigratedContract(t, filepath.Join(repo, "internal", "app", "rogue_write.go"), "package app\n")
		}},
		{name: "current-change", write: func(t *testing.T, repo string) {
			mustWriteForMigratedContract(t, filepath.Join(repo, "docs", "changes", "10-当前需求", "spec.md"), "# 被错误改写\n")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := gitRepoForMigratedContract(t)
			mustChangeForMigratedContract(t, repo, "10-当前需求")
			runGitForMigratedContract(t, repo, "add", ".")
			runGitForMigratedContract(t, repo, "commit", "-m", "current change baseline")
			head, diff, err := gitSnapshot(repo)
			if err != nil {
				t.Fatal(err)
			}
			state := migratedContractState("running-demand-guard-"+tc.name, "10-当前需求", "execution", head, diff)
			if err := saveState(repo, state); err != nil {
				t.Fatal(err)
			}
			tc.write(t, repo)
			engine := NewEngine(repo, migratedContractRegistry())
			if err := engine.detectManualIntervention(&state); err == nil {
				t.Fatal("protected current-run path should abort")
			}
			final, err := loadState(repo, state.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if final.Status != statusAborted {
				t.Fatalf("status = %q, want %q", final.Status, statusAborted)
			}
		})
	}
}

// TestDetectManualInterventionBlocksCurrentChangeRename verifies rename deltas include the protected old path.
func TestDetectManualInterventionBlocksCurrentChangeRename(t *testing.T) {
	cases := []struct {
		name  string
		write func(t *testing.T, repo string)
	}{
		{name: "staged", write: func(t *testing.T, repo string) {
			runGitForMigratedContract(t, repo, "mv", "docs/changes/10-当前需求/spec.md", "docs/changes/11-运行中新需求/stolen.md")
		}},
		{name: "committed", write: func(t *testing.T, repo string) {
			runGitForMigratedContract(t, repo, "mv", "docs/changes/10-当前需求/spec.md", "docs/changes/11-运行中新需求/stolen.md")
			runGitForMigratedContract(t, repo, "commit", "-m", "move current change into unrelated demand")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := gitRepoForMigratedContract(t)
			mustChangeForMigratedContract(t, repo, "10-当前需求")
			mustWriteForMigratedContract(t, filepath.Join(repo, "docs", "changes", "10-当前需求", "spec.md"), "# 当前规格\n")
			mustChangeForMigratedContract(t, repo, "11-运行中新需求")
			runGitForMigratedContract(t, repo, "add", ".")
			runGitForMigratedContract(t, repo, "commit", "-m", "current change baseline")
			head, diff, err := gitSnapshot(repo)
			if err != nil {
				t.Fatal(err)
			}
			state := migratedContractState("running-demand-rename-"+tc.name, "10-当前需求", "execution", head, diff)
			if err := saveState(repo, state); err != nil {
				t.Fatal(err)
			}
			tc.write(t, repo)
			engine := NewEngine(repo, migratedContractRegistry())
			if err := engine.detectManualIntervention(&state); err == nil {
				t.Fatal("renaming current change into unrelated change should abort")
			}
			final, err := loadState(repo, state.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if final.Status != statusAborted {
				t.Fatalf("status = %q, want %q", final.Status, statusAborted)
			}
		})
	}
}

// migratedContractState creates a sealed running state for manual-intervention regression tests.
func migratedContractState(runID, changeName, stage, head, diff string) State {
	return State{
		RunID:        runID,
		ChangeName:   changeName,
		Sealed:       true,
		Status:       statusRunning,
		Stage:        stage,
		BaselineHead: head,
		BaselineDiff: diff,
		Sessions:     map[string]string{},
		Stages:       map[string]string{},
		Workflow:     DefaultWorkflowConfig(),
	}
}

// gitRepoForMigratedContract creates a temporary repository with one initial commit.
func gitRepoForMigratedContract(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGitForMigratedContract(t, repo, "init")
	runGitForMigratedContract(t, repo, "config", "user.email", "test@example.com")
	runGitForMigratedContract(t, repo, "config", "user.name", "Test")
	mustWriteForMigratedContract(t, filepath.Join(repo, "README.md"), "test\n")
	runGitForMigratedContract(t, repo, "add", ".")
	runGitForMigratedContract(t, repo, "commit", "-m", "init")
	return repo
}

// mustChangeForMigratedContract writes the minimal oz change files used by state tests.
func mustChangeForMigratedContract(t *testing.T, repo, name string) {
	t.Helper()
	base := filepath.Join(repo, "docs", "changes", name)
	mustWriteForMigratedContract(t, filepath.Join(base, "brief.md"), "# "+name+"\n")
	mustWriteForMigratedContract(t, filepath.Join(base, "task.md"), "- [ ] task\n")
}

// mustWriteForMigratedContract writes a test file and creates parent directories.
func mustWriteForMigratedContract(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// runGitForMigratedContract runs git commands in a test repository.
func runGitForMigratedContract(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

// migratedContractRegistry returns an agent registry that never calls external CLIs.
func migratedContractRegistry() *AgentRegistry {
	registry := &AgentRegistry{}
	registry.Register(migratedContractTool{name: "codex"})
	registry.Register(migratedContractTool{name: "pi"})
	registry.Register(migratedContractTool{name: "agy"})
	return registry
}

type migratedContractTool struct {
	name string
}

// Name returns the configured backend name used by test workflow snapshots.
func (t migratedContractTool) Name() string { return t.name }

// Resolve keeps tests independent from real agent CLI binaries.
func (t migratedContractTool) Resolve() error { return nil }

// PlanningCommand is not used by these sealed-run regression tests.
func (t migratedContractTool) PlanningCommand(context.Context, string, string, io.Reader, StageOptions) (*exec.Cmd, error) {
	return nil, os.ErrInvalid
}

// NewRunner returns a no-op runner for sealed-run guard checks.
func (t migratedContractTool) NewRunner() AgentRunner {
	return migratedContractRunner{}
}

type migratedContractRunner struct{}

// Run returns a stable session id without touching real agent backends.
func (migratedContractRunner) Run(context.Context, string, string, string, StageOptions) (string, error) {
	return "migrated-contract-session", nil
}
