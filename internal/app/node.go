// Package app contains node helpers used by the built-in Go DAG scheduler.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	done, err = e.nodeStageDone(state)
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
	if !done {
		err := e.stageArtifactGateError(state, fmt.Errorf("%s 阶段 artifact 未完成", stage))
		if recordErr := recordStageArtifactGateFailure(e.Repo, &state, err); recordErr != nil {
			return recordErr
		}
		if saveErr := saveState(e.Repo, state); saveErr != nil {
			return saveErr
		}
		return err
	}
	clearStageArtifactGateFailure(&state)
	if stage == "execution" {
		preflightPassed, err := e.runAcceptancePreflight(&state)
		if err != nil {
			return e.failNodeState(state, err)
		}
		if !preflightPassed {
			if err := saveState(e.Repo, state); err != nil {
				return err
			}
			return fmt.Errorf("%s acceptance preflight 未通过", stage)
		}
	}
	validationPassed, err := e.validateStage(ctx, &state)
	if err != nil {
		return e.failNodeState(state, err)
	}
	if !validationPassed {
		if err := saveState(e.Repo, state); err != nil {
			return err
		}
		return fmt.Errorf("%s validation 未通过", stage)
	}
	markStageCompleted(&state)
	if stage == "execution" || strings.HasPrefix(stage, "fix_") || stage == "archive" {
		if err := e.advance(&state); err != nil {
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

// nodeFanin combines all member artifacts into the existing parallel schema.
func (e *Engine) nodeFanin(state State, args []string, stdout io.Writer) error {
	groupName, err := requireFlagValue(args, "--group")
	if err != nil {
		return err
	}
	stage, err := requireFlagValue(args, "--stage")
	if err != nil {
		return err
	}
	iteration, err := nodeIteration(args, stage)
	if err != nil {
		return e.failNodeState(state, err)
	}
	if state.Status != statusRunning || state.Stage != stage {
		return writeNodeResult(stdout, nodeResult{Status: "skipped", RunID: state.RunID, Stage: stage, Group: groupName})
	}
	configName := configGroupName(groupName)
	group, ok := state.Workflow.Parallel.Groups[configName]
	if !state.Workflow.Parallel.Enabled || !ok {
		return writeNodeResult(stdout, nodeResult{Status: "skipped", RunID: state.RunID, Stage: stage, Group: groupName})
	}
	artifact := ParallelArtifact{Group: configName, Mode: group.Mode, Summary: configName + " fanin completed"}
	for _, member := range group.Members {
		path := memberArtifactPath(e.Repo, state.RunID, configName, iteration, member.Name)
		result, err := readNormalizeValidateMemberArtifact(path, configName, member, state.ChangeName)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				artifact.Members = append(artifact.Members, missingParallelMemberResult(member, state.ChangeName, path))
				continue
			}
			return e.failNodeState(state, err)
		}
		if err := writeMemberArtifact(path, result); err != nil {
			return e.failNodeState(state, err)
		}
		artifact.Members = append(artifact.Members, result)
	}
	if err := ValidateParallelArtifact(artifact); err != nil {
		return e.failNodeState(state, err)
	}
	if err := ValidateParallelArtifactForGroup(artifact, configName, group); err != nil {
		return e.failNodeState(state, err)
	}
	path := parallelArtifactPath(runDir(e.Repo, state.RunID), configName, iteration)
	if err := writeJSONFile(path, artifact); err != nil {
		return e.failNodeState(state, err)
	}
	return writeNodeResult(stdout, nodeResult{Status: "completed", RunID: state.RunID, Stage: stage, Group: groupName, Artifact: path})
}

// missingParallelMemberResult records absent helper output without blocking the main workflow.
func missingParallelMemberResult(member ParallelMemberConfig, changeName, path string) ParallelMemberResult {
	return ParallelMemberResult{
		Name:       member.Name,
		ChangeName: changeName,
		Purpose:    member.Purpose,
		Status:     "missing",
		Summary:    "helper artifact missing; main stage should proceed with remaining context",
		Evidence:   []string{"missing artifact: " + path},
		Required:   member.Required,
	}
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
