// Package app contains node helpers used by the built-in Go DAG scheduler.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type nodeResult struct {
	Status   string `json:"status"`
	RunID    string `json:"run_id"`
	Stage    string `json:"stage,omitempty"`
	Group    string `json:"group,omitempty"`
	Member   string `json:"member,omitempty"`
	Artifact string `json:"artifact,omitempty"`
}

// nodeRunStage runs one activated main stage and validates its artifact.
func (e *Engine) nodeRunStage(ctx context.Context, state State, args []string, stdout io.Writer) error {
	stage, err := requireFlagValue(args, "--stage")
	if err != nil {
		return err
	}
	if state.Status != statusRunning || state.Stage != stage {
		return writeNodeResult(stdout, nodeResult{Status: "skipped", RunID: state.RunID, Stage: stage})
	}
	forceRun := shouldForceStageRerun(state)
	done, err := e.nodeStageDone(state)
	if err != nil && !isStageArtifactGateError(err) {
		return e.failNodeState(state, err)
	}
	if !done || forceRun {
		if err := e.detectManualIntervention(&state); err != nil {
			return err
		}
		if err := e.runStage(ctx, &state); err != nil {
			return e.failNodeState(state, err)
		}
	}
	result, err := e.completeMainStage(ctx, &state, stageGatePipelineNode)
	if err != nil {
		if isStageArtifactGateError(err) {
			if recordErr := recordStageArtifactGateFailure(e.Repo, &state, err); recordErr != nil {
				return recordErr
			}
			if saveErr := saveState(e.Repo, state); saveErr != nil {
				return saveErr
			}
			return err
		}
		return e.failNodeState(state, err)
	}
	if err := nodeStageGateError(stage, result); err != nil {
		if saveErr := saveState(e.Repo, state); saveErr != nil {
			return saveErr
		}
		return err
	}
	if err := saveState(e.Repo, state); err != nil {
		return err
	}
	return writeNodeResult(stdout, nodeResult{Status: "completed", RunID: state.RunID, Stage: stage})
}

// nodeStageDone checks stage-local output before advancing scheduler gates.
func (e *Engine) nodeStageDone(state State) (bool, error) {
	return e.checkStageArtifactGate(state)
}

// nodeGate advances durable workflow state after a completed stage.
func (e *Engine) nodeGate(state State, args []string, stdout io.Writer) error {
	stage, err := requireFlagValue(args, "--stage")
	if err != nil {
		return err
	}
	if state.Status != statusRunning || state.Stage != stage {
		return writeNodeResult(stdout, nodeResult{Status: "skipped", RunID: state.RunID, Stage: stage})
	}
	done, err := e.artifactDone(state)
	if err != nil {
		return e.failNodeState(state, err)
	}
	if !done {
		if stage == "archive" {
			return writeNodeResult(stdout, nodeResult{Status: "skipped", RunID: state.RunID, Stage: stage})
		}
		return e.failNodeState(state, fmt.Errorf("%s gate 前 artifact 未完成", stage))
	}
	if err := e.advance(&state); err != nil {
		return e.failNodeState(state, err)
	}
	if err := saveState(e.Repo, state); err != nil {
		return err
	}
	return writeNodeResult(stdout, nodeResult{Status: "completed", RunID: state.RunID, Stage: stage})
}

// failNodeState records node failures in durable run state.
func (e *Engine) failNodeState(state State, err error) error {
	state = failedState(state, err)
	_ = mergeState(e.Repo, state.RunID, func(latest *State) {
		latest.Status = state.Status
		if latest.Error == "" {
			latest.Error = state.Error
		}
	})
	return err
}

func writeJSONFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func writeNodeResult(stdout io.Writer, result nodeResult) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	_, err = stdout.Write(append(data, '\n'))
	return err
}
