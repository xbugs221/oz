// Package app verifies workflow-scoped agent dependency resolution.
package app

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"reflect"
	"testing"
)

// dependencyProbeTool records whether dependency resolution inspected a backend.
type dependencyProbeTool struct {
	name     string
	resolved *[]string
	err      error
}

// Name returns the backend name used in workflow snapshots.
func (t dependencyProbeTool) Name() string { return t.name }

// Resolve records the dependency check and returns its configured result.
func (t dependencyProbeTool) Resolve() error {
	*t.resolved = append(*t.resolved, t.name)
	return t.err
}

// PlanningCommand is unused by dependency-resolution tests.
func (t dependencyProbeTool) PlanningCommand(context.Context, string, string, io.Reader, StageOptions) (*exec.Cmd, error) {
	return nil, errors.New("not implemented")
}

// NewRunner is unused by dependency-resolution tests.
func (t dependencyProbeTool) NewRunner() AgentRunner { return nil }

// TestResolveForWorkflowChecksOnlySnapshotTools verifies default snapshots require only Codex.
func TestResolveForWorkflowChecksOnlySnapshotTools(t *testing.T) {
	resolved := []string{}
	registry := &AgentRegistry{tools: map[string]AgentTool{}}
	for _, name := range []string{"codex", "pi", "agy"} {
		registry.Register(dependencyProbeTool{name: name, resolved: &resolved})
	}

	if err := registry.ResolveForWorkflow(DefaultWorkflowConfig()); err != nil {
		t.Fatalf("resolve default workflow: %v", err)
	}
	if want := []string{"codex"}; !reflect.DeepEqual(resolved, want) {
		t.Fatalf("resolved tools = %v, want %v", resolved, want)
	}
}

// TestResolveForWorkflowChecksEachConfiguredTool verifies mixed snapshots are deduplicated.
func TestResolveForWorkflowChecksEachConfiguredTool(t *testing.T) {
	resolved := []string{}
	registry := &AgentRegistry{tools: map[string]AgentTool{}}
	for _, name := range []string{"codex", "pi", "agy"} {
		registry.Register(dependencyProbeTool{name: name, resolved: &resolved})
	}
	workflow := DefaultWorkflowConfig()
	options := workflow.Stages["qa"]
	options.Tool = "pi"
	workflow.Stages["qa"] = options

	if err := registry.ResolveForWorkflow(workflow); err != nil {
		t.Fatalf("resolve mixed workflow: %v", err)
	}
	if want := []string{"codex", "pi"}; !reflect.DeepEqual(resolved, want) {
		t.Fatalf("resolved tools = %v, want %v", resolved, want)
	}
}
