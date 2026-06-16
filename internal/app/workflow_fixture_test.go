// Package app provides shared workflow test fixtures for DAG and gate tests.
package app

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// workflowFixture owns a temporary repository and common fake workflow dependencies.
type workflowFixture struct {
	t      *testing.T
	repo   string
	runner *fakeWorkflowRunner
}

// newWorkflowFixture creates a temp repository fixture for workflow tests.
func newWorkflowFixture(t *testing.T) *workflowFixture {
	t.Helper()
	return &workflowFixture{
		t:      t,
		repo:   t.TempDir(),
		runner: &fakeWorkflowRunner{},
	}
}

// writeActiveChange creates the minimum active change files used by workflow tests.
func (f *workflowFixture) writeActiveChange(name string) {
	f.t.Helper()
	changeDir := filepath.Join(f.repo, "docs", "changes", name)
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(changeDir, "task.md"), []byte("- [ ] fixture task\n"), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// writeAcceptanceContract writes a minimal acceptance contract for gate tests.
func (f *workflowFixture) writeAcceptanceContract(changeName string, body string) {
	f.t.Helper()
	changeDir := filepath.Join(f.repo, "docs", "changes", changeName)
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(changeDir, "acceptance.json"), []byte(body), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// fakeToolRegistry returns a one-tool registry backed by the fixture runner.
func (f *workflowFixture) fakeToolRegistry() *AgentRegistry {
	f.t.Helper()
	registry := NewAgentRegistry()
	registry.Register(fakeWorkflowTool{runner: f.runner})
	return registry
}

// git runs a repository-scoped git command for tests that need real commits.
func (f *workflowFixture) git(args ...string) {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = f.repo
	if out, err := cmd.CombinedOutput(); err != nil {
		f.t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}

type fakeWorkflowTool struct {
	runner AgentRunner
}

// Name returns the fake tool name used in workflow configs.
func (fakeWorkflowTool) Name() string { return "codex" }

// Resolve confirms the fake tool is available.
func (fakeWorkflowTool) Resolve() error { return nil }

// PlanningCommand is unused by these fixtures.
func (fakeWorkflowTool) PlanningCommand(context.Context, string, string, io.Reader, StageOptions) (*exec.Cmd, error) {
	return nil, nil
}

// NewRunner returns the fixture runner.
func (t fakeWorkflowTool) NewRunner() AgentRunner { return t.runner }

// fakeWorkflowRunner aliases the existing DAG runner behavior for shared fixture users.
type fakeWorkflowRunner struct {
	goDAGContextFakeRunner
}

// TestWorkflowFixtureWritesChangeAndRegistry verifies the shared fixture creates reusable workflow boundaries.
func TestWorkflowFixtureWritesChangeAndRegistry(t *testing.T) {
	fixture := newWorkflowFixture(t)
	fixture.writeActiveChange("1-demo")
	fixture.writeAcceptanceContract("1-demo", `{"summary":"fixture","coverage":[]}`)
	registry := fixture.fakeToolRegistry()

	if _, err := os.Stat(filepath.Join(fixture.repo, "docs", "changes", "1-demo", "task.md")); err != nil {
		t.Fatalf("fixture did not write task.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fixture.repo, "docs", "changes", "1-demo", "acceptance.json")); err != nil {
		t.Fatalf("fixture did not write acceptance.json: %v", err)
	}
	if _, err := registry.Tool("codex"); err != nil {
		t.Fatalf("fixture registry missing fake tool: %v", err)
	}
}
