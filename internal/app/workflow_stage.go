// Package app defines workflow stage and status semantics shared by engine paths.
package app

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	workflowStageExecution = "execution"
	workflowStagePlanning  = "planning"
	workflowStageReview    = "review"
	workflowStageFix       = "fix"
	workflowStageQA        = "qa"
	workflowStageArchive   = "archive"
	workflowStageDone      = "done"
)

// workflowStage is the single parsed representation of a durable stage string.
type workflowStage struct {
	Raw       string
	Kind      string
	Iteration int
	Iterable  bool
}

// parseWorkflowStage converts public durable stage strings into internal semantics.
func parseWorkflowStage(stage string) (workflowStage, error) {
	switch stage {
	case workflowStagePlanning, workflowStageExecution, workflowStageArchive, workflowStageDone:
		return workflowStage{Raw: stage, Kind: stage}, nil
	}
	for _, spec := range []struct {
		prefix string
		kind   string
	}{
		{prefix: "review_", kind: workflowStageReview},
		{prefix: "fix_", kind: workflowStageFix},
		{prefix: "qa_", kind: workflowStageQA},
	} {
		if strings.HasPrefix(stage, spec.prefix) {
			raw := strings.TrimPrefix(stage, spec.prefix)
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 {
				return workflowStage{}, fmt.Errorf("非法迭代阶段 %q", stage)
			}
			return workflowStage{Raw: stage, Kind: spec.kind, Iteration: n, Iterable: true}, nil
		}
	}
	return workflowStage{}, fmt.Errorf("未知阶段 %q", stage)
}

// isKind reports whether this parsed stage belongs to the requested stage kind.
func (s workflowStage) isKind(kind string) bool {
	return s.Kind == kind
}

// runStatus groups durable status strings while preserving public JSON values.
type runStatus string

// normalizeRunStatus converts aliases and legacy values into canonical run status.
func normalizeRunStatus(status string) runStatus {
	switch status {
	case "", statusRunning:
		return runStatus(statusRunning)
	case "success", "completed", statusDone:
		return runStatus(statusDone)
	case statusFailed, "error", "validation_failed":
		return runStatus(statusFailed)
	default:
		return runStatus(status)
	}
}

// isRunning reports whether the run can still advance.
func (s runStatus) isRunning() bool {
	return s == runStatus(statusRunning)
}

// isDone reports whether the run completed successfully.
func (s runStatus) isDone() bool {
	return s == runStatus(statusDone)
}

// isTerminal reports whether this status should stop workflow execution.
func (s runStatus) isTerminal() bool {
	return s != "" && !s.isRunning()
}

// isBlocked reports whether this status represents a deliberate workflow gate stop.
func (s runStatus) isBlocked() bool {
	switch string(s) {
	case statusBlocked, statusValidationBlocked, statusAcceptanceContractBlocked:
		return true
	default:
		return false
	}
}
