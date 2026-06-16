// Package app centralizes deterministic gates that complete one main workflow stage.
package app

import (
	"context"
	"fmt"
)

type stageGatePipelineMode string

const (
	stageGatePipelineLoop stageGatePipelineMode = "loop"
	stageGatePipelineNode stageGatePipelineMode = "node"
)

// stageGatePipelineResult describes the caller-visible outcome of completing a stage.
type stageGatePipelineResult struct {
	Done          bool
	Blocked       bool
	ProgressLabel string
}

// completeMainStage runs artifact, acceptance, validation, completion, and advance gates once.
func (e *Engine) completeMainStage(ctx context.Context, state *State, mode stageGatePipelineMode) (stageGatePipelineResult, error) {
	done, err := e.checkStageArtifactGate(*state)
	if err != nil {
		return stageGatePipelineResult{}, err
	}
	if !done {
		return stageGatePipelineResult{}, nil
	}
	clearStageArtifactGateFailure(state)
	if state.Stage == workflowStageExecution {
		preflightPassed, err := e.runAcceptancePreflight(state)
		if err != nil {
			return stageGatePipelineResult{}, err
		}
		if !preflightPassed {
			return stageGatePipelineResult{Done: true, Blocked: true, ProgressLabel: "blocked"}, nil
		}
	}
	acceptancePassed, err := e.runAcceptanceGate(ctx, state)
	if err != nil {
		return stageGatePipelineResult{}, err
	}
	if !acceptancePassed {
		return stageGatePipelineResult{Done: true, Blocked: true, ProgressLabel: failedGateProgressLabel(*state)}, nil
	}
	validationPassed, err := e.validateStage(ctx, state)
	if err != nil {
		return stageGatePipelineResult{}, err
	}
	if !validationPassed {
		return stageGatePipelineResult{Done: true, Blocked: true, ProgressLabel: failedGateProgressLabel(*state)}, nil
	}
	if !normalizeRunStatus(state.Status).isRunning() {
		return stageGatePipelineResult{Done: true, Blocked: true, ProgressLabel: "blocked"}, nil
	}
	markStageCompleted(state)
	if shouldAdvanceAfterMainStage(*state, mode) {
		if err := e.advance(state); err != nil {
			return stageGatePipelineResult{}, err
		}
	}
	return stageGatePipelineResult{Done: true, ProgressLabel: "next"}, nil
}

// shouldAdvanceAfterMainStage preserves loop and DAG scheduling boundaries.
func shouldAdvanceAfterMainStage(state State, mode stageGatePipelineMode) bool {
	if mode == stageGatePipelineLoop {
		return true
	}
	stage, err := parseWorkflowStage(state.Stage)
	if err != nil {
		return false
	}
	return stage.isKind(workflowStageExecution) || stage.isKind(workflowStageFix) || stage.isKind(workflowStageArchive)
}

// failedGateProgressLabel maps persisted gate status to the existing progress vocabulary.
func failedGateProgressLabel(state State) string {
	if state.Status == statusAcceptanceContractBlocked || state.Status == statusValidationBlocked {
		return "blocked"
	}
	return "validation_failed"
}

// nodeStageGateError converts a pipeline stop into the node contract's error style.
func nodeStageGateError(stage string, result stageGatePipelineResult) error {
	if !result.Done {
		return fmt.Errorf("%s 阶段 artifact 未完成", stage)
	}
	if result.Blocked {
		if result.ProgressLabel == "blocked" {
			return fmt.Errorf("%s gate blocked", stage)
		}
		return fmt.Errorf("%s validation 未通过", stage)
	}
	return nil
}
