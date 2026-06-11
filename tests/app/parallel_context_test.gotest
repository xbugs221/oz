// Package app tests parallel planning and implementation context gates.
package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestArtifactDoneRequiresParallelPlanningContextWhenEnabled verifies planning context is not dead config.
func TestArtifactDoneRequiresParallelPlanningContextWhenEnabled(t *testing.T) {
	// Given: execution tasks and implementation context are done, but planning context is missing.
	repo := gitRepo(t)
	runID := "parallel-planning-context-missing-run"
	base := runDir(repo, runID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	writeDoneTask(t, repo)
	mustWritePrompt(t, filepath.Join(base, "parallel-implementation-context.json"), parallelContextArtifactForTest())
	state := State{RunID: runID, ChangeName: "demo", Stage: "execution", Status: statusRunning, Workflow: parallelWorkflowForTest()}

	// When: the state machine checks whether execution is complete.
	done, err := (&Engine{Repo: repo}).artifactDone(state)

	// Then: completion is rejected until parallel-planning-context.json is present.
	if err == nil || !strings.Contains(err.Error(), "parallel-planning-context.json") {
		t.Fatalf("artifactDone should require planning context, done=%v err=%v", done, err)
	}
}
