// Package app tests the shared main-stage gate pipeline boundary.
package app

import (
	"strings"
	"testing"
)

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
