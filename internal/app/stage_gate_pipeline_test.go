// Package app tests the shared main-stage gate pipeline boundary.
package app

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestExecutionArtifactGateUsesRuntimeState verifies execution completion never depends on a Git task file.
func TestExecutionArtifactGateUsesRuntimeState(t *testing.T) {
	repo := t.TempDir()
	engine := NewEngine(repo, NewAgentRegistry())
	state := State{
		RunID:      "no-task-file",
		ChangeName: "47-无任务文件",
		Stage:      "execution",
		Stages:     map[string]string{"execution": stageStatusAgentCompleted},
	}

	expectation := engine.stageArtifactExpectation(state)
	if expectation.Path != filepath.Join(runDir(repo, state.RunID), "state.json") {
		t.Fatalf("execution artifact = %q, want runtime state", expectation.Path)
	}
	if strings.Contains(expectation.Path+expectation.Description, "task.md") {
		t.Fatalf("execution gate still mentions task.md: %#v", expectation)
	}
	pending := state
	pending.Stages = map[string]string{}
	if _, done, err := engine.validateStageArtifact(pending); err != nil || done {
		t.Fatalf("execution must not be skipped before the agent returns: done=%v err=%v", done, err)
	}
	if _, done, err := engine.validateStageArtifact(state); err != nil || !done {
		t.Fatalf("successful execution return must satisfy fileless artifact gate: done=%v err=%v", done, err)
	}
}

// TestFailedGateProgressLabelCoversAcceptanceAndValidationFailures proves blocked/retry labels stay compatible.
func TestFailedGateProgressLabelCoversAcceptanceAndValidationFailures(t *testing.T) {
	tests := []struct {
		name   string
		state  State
		expect string
	}{
		{name: "acceptance retry", state: State{Status: statusRunning}, expect: "validation_failed"},
		{name: "acceptance blocked", state: State{Status: statusAcceptanceContractBlocked}, expect: "blocked"},
		{name: "validation retry", state: State{Status: statusFailed}, expect: "validation_failed"},
		{name: "validation blocked", state: State{Status: statusValidationBlocked}, expect: "blocked"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := failedGateProgressLabel(tt.state); got != tt.expect {
				t.Fatalf("failedGateProgressLabel(%q) = %q, want %q", tt.state.Status, got, tt.expect)
			}
		})
	}
}

// TestShouldAdvanceAfterMainStagePreservesDAGSchedulerGates proves archive clean stages advance in node mode.
func TestShouldAdvanceAfterMainStagePreservesDAGSchedulerGates(t *testing.T) {
	tests := []struct {
		name   string
		state  State
		mode   stageGatePipelineMode
		expect bool
	}{
		{name: "loop advances review", state: State{Stage: "review_1"}, mode: stageGatePipelineLoop, expect: true},
		{name: "node advances repair", state: State{Stage: "repair_1"}, mode: stageGatePipelineNode, expect: true},
		{name: "node leaves review to nodeGate", state: State{Stage: "review_1"}, mode: stageGatePipelineNode, expect: false},
		{name: "node leaves qa to nodeGate", state: State{Stage: "qa_1"}, mode: stageGatePipelineNode, expect: false},
		{name: "node advances execution", state: State{Stage: "execution"}, mode: stageGatePipelineNode, expect: true},
		{name: "node advances fix", state: State{Stage: "fix_1"}, mode: stageGatePipelineNode, expect: true},
		{name: "node advances archive clean", state: State{Stage: "archive"}, mode: stageGatePipelineNode, expect: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldAdvanceAfterMainStage(tt.state, tt.mode); got != tt.expect {
				t.Fatalf("shouldAdvanceAfterMainStage(%s, %s) = %v, want %v", tt.state.Stage, tt.mode, got, tt.expect)
			}
		})
	}
}

// TestNodeStageGateErrorCoversPipelineStops proves node mode exposes failed pipeline results as errors.
func TestNodeStageGateErrorCoversPipelineStops(t *testing.T) {
	tests := []struct {
		name   string
		result stageGatePipelineResult
		want   string
	}{
		{name: "missing artifact", result: stageGatePipelineResult{}, want: "artifact"},
		{name: "acceptance blocked", result: stageGatePipelineResult{Done: true, Blocked: true, ProgressLabel: "blocked"}, want: "gate blocked"},
		{name: "validation failed", result: stageGatePipelineResult{Done: true, Blocked: true, ProgressLabel: "validation_failed"}, want: "validation"},
		{name: "passed", result: stageGatePipelineResult{Done: true, ProgressLabel: "next"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := nodeStageGateError("execution", tt.result)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("nodeStageGateError returned unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("nodeStageGateError returned nil error, want %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("nodeStageGateError = %q, want substring %q", err.Error(), tt.want)
			}
		})
	}
}
