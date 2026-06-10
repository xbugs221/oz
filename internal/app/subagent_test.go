// Package app tests parallel subagent node execution behavior.
package app

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// TestNodeRunSubagentReturnsMergeStateError verifies completion sessions are required state.
func TestNodeRunSubagentReturnsMergeStateError(t *testing.T) {
	repo := gitRepo(t)
	runID := "run-subagent-merge-error"
	workflow := DefaultWorkflowConfig()
	group := workflow.Parallel.Groups["implementation_context"]
	if len(group.Members) == 0 {
		t.Fatal("implementation_context group missing test member")
	}
	member := group.Members[0]
	state := State{
		RunID:      runID,
		ChangeName: "demo",
		Status:     statusRunning,
		Stage:      "execution",
		Sessions:   map[string]string{"codex:executor": "executor-session"},
		Workflow:   workflow,
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}

	originalMerge := mergeSubagentSessionState
	mergeSubagentSessionState = func(string, string, func(*State)) error {
		return errors.New("state write failed")
	}
	defer func() { mergeSubagentSessionState = originalMerge }()

	engine := Engine{Repo: repo, Registry: testRegistry(fakeRunner{})}
	var stdout bytes.Buffer
	err := engine.nodeRunSubagent(context.Background(), state, []string{"--group", "implementation_context", "--member", member.Name, "--stage", "execution"}, &stdout)
	if err == nil || !strings.Contains(err.Error(), "record subagent session") {
		t.Fatalf("nodeRunSubagent error = %v, want record subagent session failure", err)
	}
	if strings.Contains(stdout.String(), `"status":"completed"`) {
		t.Fatalf("subagent node reported completed despite merge failure:\n%s", stdout.String())
	}

	latest, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Status != statusFailed {
		t.Fatalf("state status = %q, want failed after session merge failure", latest.Status)
	}
	options, err := workflow.StageOption("execution")
	if err != nil {
		t.Fatal(err)
	}
	tool := options.Tool
	if member.Tool != "" {
		tool = member.Tool
	}
	key := sessionStateKey(tool, "subagent:implementation_context:"+member.Name+":0")
	if latest.Sessions[key] != "" {
		t.Fatalf("session %s persisted despite merge failure: %q", key, latest.Sessions[key])
	}
}
