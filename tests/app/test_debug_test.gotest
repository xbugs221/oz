package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestQAArtifactGateDebug(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  workflow:\n    engine: go-dag\n    max_review_iterations: 1\n    parallel:\n      enabled: false\n")
	runner := &qaArtifactRepairRunner{}
	engine := NewEngine(repo, testRegistry(runner))

	// monkey-patch debugging
	origNodeRunStage := engine.nodeRunStage
	_ = origNodeRunStage

	err := engine.Start(context.Background(), "demo")
	t.Logf("stages: %s", strings.Join(runner.stages, ","))
	t.Logf("qaAttempts: %d", runner.qaAttempts)
	t.Logf("error: %v", err)

	// check state
	runID, _ := newestRun(repo)
	state, _ := loadState(repo, runID)
	t.Logf("state.Status: %s", state.Status)
	t.Logf("state.Stage: %s", state.Stage)
	if v, ok := state.Validation["qa_1"]; ok {
		t.Logf("validation qa_1: Attempts=%d Status=%s", v.Attempts, v.Status)
	} else {
		t.Logf("validation qa_1: missing")
	}
	if s, ok := state.Stages["qa_1"]; ok {
		t.Logf("stages qa_1: %s", s)
	} else {
		t.Logf("stages qa_1: missing")
	}
}
