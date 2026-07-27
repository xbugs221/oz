// Package app tests durable execution completion across post-agent infrastructure failures.
package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

type countingExecutionRunner struct {
	calls int
}

// Run records one successful agent turn for persistence and recovery assertions.
func (r *countingExecutionRunner) Run(context.Context, string, string, string, StageOptions) (string, error) {
	r.calls++
	return "execution-session", nil
}

// TestExecutionCompletionPersistsBeforeGitSnapshotFailure proves resume skips a successful agent turn.
func TestExecutionCompletionPersistsBeforeGitSnapshotFailure(t *testing.T) {
	repo := gitRepo(t)
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	fixture := &workflowFixture{t: t, repo: repo}
	changeName := "1-执行完成恢复"
	fixture.writeActiveChange(changeName)
	writeAcceptanceRunChange(
		t,
		repo,
		changeName,
		[]acceptanceRunFixtureTest{{id: "execution-completion", body: "true"}},
		nil,
	)

	runner := &countingExecutionRunner{}
	registry := NewAgentRegistry()
	registry.Register(fakeWorkflowTool{runner: runner})
	engine := NewEngine(repo, registry)
	head, diff, err := gitSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	state := State{
		RunID:                   newRunID(),
		ChangeName:              changeName,
		Sealed:                  true,
		ProposalContractVersion: currentProposalContractVersion,
		Status:                  statusRunning,
		Stage:                   workflowStageExecution,
		BaselineHead:            head,
		BaselineDiff:            diff,
		Sessions:                map[string]string{},
		Stages:                  map[string]string{},
		Paths:                   map[string]string{},
		Workflow:                DefaultWorkflowConfig(),
	}
	if err := snapshotRunPrompts(repo, state.RunID); err != nil {
		t.Fatal(err)
	}
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptancePath(repo, changeName)); err != nil {
		t.Fatal(err)
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	engine.stageGitSnapshot = func(string) (string, string, error) {
		return "", "", errors.New("injected git snapshot failure")
	}

	if err := engine.runStage(context.Background(), &state); err == nil {
		t.Fatal("expected injected post-agent snapshot failure")
	}
	recovered, err := loadState(repo, state.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Stages[workflowStageExecution] != stageStatusAgentCompleted {
		t.Fatalf("persisted execution marker = %q", recovered.Stages[workflowStageExecution])
	}
	done, err := engine.artifactDone(recovered)
	if err != nil || !done {
		t.Fatalf("recovered execution artifact = %v, %v; want done", done, err)
	}
	if runner.calls != 1 {
		t.Fatalf("execution runner calls = %d, want 1", runner.calls)
	}
}
