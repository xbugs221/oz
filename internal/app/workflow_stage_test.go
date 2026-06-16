// Package app tests shared workflow stage and status semantics.
package app

import "testing"

// TestParseWorkflowStageCoversPublicStageStrings proves durable stage strings keep one parser.
func TestParseWorkflowStageCoversPublicStageStrings(t *testing.T) {
	tests := []struct {
		name      string
		stage     string
		kind      string
		iteration int
		iterable  bool
		wantErr   bool
	}{
		{name: "planning", stage: "planning", kind: workflowStagePlanning},
		{name: "execution", stage: "execution", kind: workflowStageExecution},
		{name: "review", stage: "review_2", kind: workflowStageReview, iteration: 2, iterable: true},
		{name: "qa", stage: "qa_3", kind: workflowStageQA, iteration: 3, iterable: true},
		{name: "fix", stage: "fix_4", kind: workflowStageFix, iteration: 4, iterable: true},
		{name: "archive", stage: "archive", kind: workflowStageArchive},
		{name: "done", stage: "done", kind: workflowStageDone},
		{name: "bad iteration", stage: "review_zero", wantErr: true},
		{name: "zero iteration", stage: "qa_0", wantErr: true},
		{name: "unknown", stage: "deploy", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseWorkflowStage(tt.stage)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseWorkflowStage(%q) returned nil error", tt.stage)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseWorkflowStage(%q) returned error: %v", tt.stage, err)
			}
			if got.Raw != tt.stage || got.Kind != tt.kind || got.Iteration != tt.iteration || got.Iterable != tt.iterable {
				t.Fatalf("parsed stage = %#v, want raw=%q kind=%q iteration=%d iterable=%v", got, tt.stage, tt.kind, tt.iteration, tt.iterable)
			}
		})
	}
}

// TestNormalizeRunStatusGroupsPublicStrings proves status helpers keep JSON strings compatible.
func TestNormalizeRunStatusGroupsPublicStrings(t *testing.T) {
	tests := []struct {
		status   string
		want     runStatus
		running  bool
		done     bool
		terminal bool
		blocked  bool
	}{
		{status: "", want: runStatus(statusRunning), running: true},
		{status: statusRunning, want: runStatus(statusRunning), running: true},
		{status: "completed", want: runStatus(statusDone), done: true, terminal: true},
		{status: statusDone, want: runStatus(statusDone), done: true, terminal: true},
		{status: "validation_failed", want: runStatus(statusFailed), terminal: true},
		{status: statusValidationBlocked, want: runStatus(statusValidationBlocked), terminal: true, blocked: true},
		{status: statusAcceptanceContractBlocked, want: runStatus(statusAcceptanceContractBlocked), terminal: true, blocked: true},
		{status: statusInterrupted, want: runStatus(statusInterrupted), terminal: true},
		{status: statusStale, want: runStatus(statusStale), terminal: true},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := normalizeRunStatus(tt.status)
			if got != tt.want {
				t.Fatalf("normalizeRunStatus(%q) = %q, want %q", tt.status, got, tt.want)
			}
			if got.isRunning() != tt.running || got.isDone() != tt.done || got.isTerminal() != tt.terminal || got.isBlocked() != tt.blocked {
				t.Fatalf("status groups for %q = running:%v done:%v terminal:%v blocked:%v", got, got.isRunning(), got.isDone(), got.isTerminal(), got.isBlocked())
			}
		})
	}
}
