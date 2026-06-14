// Package app contains workflow engine state and execution boundaries.
package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
)

// Engine coordinates state persistence, lock ownership, and agent turns.
type Engine struct {
	Repo     string
	Registry *AgentRegistry
	Output   io.Writer

	PlanningTool      string
	PlanningSessionID string
	progressMu        sync.Mutex
	lastProgressState string
	progressLines     int
	stageRuntime      map[string]stageRuntime
	inPlaceProgress   bool
}

// stageRuntime is transient process metadata shown beside the current stage.
type stageRuntime struct {
	PID    string
	Thread string
	Exit   string
	Failed bool
}

type progressSetter interface {
	SetProgress(io.Writer)
}

var (
	currentExecutable         = os.Executable
	startDetachedCommand      = startDetachedResumeCommand
	startDetachedBatchCommand = startDetachedBatchResumeCommand
)

// NewEngine creates a workflow engine rooted at a git repository.
func NewEngine(repo string, registry *AgentRegistry) *Engine {
	if registry == nil {
		registry = NewAgentRegistry()
	}
	return &Engine{Repo: repo, Registry: registry, stageRuntime: map[string]stageRuntime{}}
}

// Start creates a sealed run for an active change and runs it to completion.
func (e *Engine) Start(ctx context.Context, changeName string) error {
	state, err := e.createRun(changeName)
	if err != nil {
		return err
	}
	return e.run(ctx, state)
}

// Submit creates a sealed run and starts a detached worker to advance it.
func (e *Engine) Submit(ctx context.Context, changeName string) error {
	_ = ctx
	state, err := e.createRun(changeName)
	if err != nil {
		return err
	}
	if err := startDetachedCommand(e.Repo, state.RunID); err != nil {
		return err
	}
	e.printProgress(state, "submitted")
	return nil
}

// StartJSON creates a sealed run, emits its runner DTO, then runs the default Go DAG engine.
func (e *Engine) StartJSON(ctx context.Context, changeName string, stdout io.Writer) error {
	return e.StartGoDAGJSON(ctx, changeName, stdout)
}

// createRun validates a change and persists the initial sealed run state.
func (e *Engine) createRun(changeName string) (State, error) {
	if err := ValidateChange(e.Repo, changeName); err != nil {
		return State{}, err
	}
	head, diff, err := gitSnapshot(e.Repo)
	if err != nil {
		return State{}, err
	}
	acceptanceSource := acceptancePath(e.Repo, changeName)
	if _, err := ReadAcceptance(acceptanceSource); err != nil {
		return State{}, err
	}
	workflow, err := LoadWorkflowConfig(e.Repo)
	if err != nil {
		return State{}, err
	}
	if err := e.Registry.ResolveForWorkflow(workflow); err != nil {
		return State{}, err
	}
	state := State{
		RunID:        newRunID(),
		ChangeName:   changeName,
		Sealed:       true,
		Status:       statusRunning,
		Stage:        "execution",
		Engine:       workflow.Engine,
		BaselineHead: head,
		BaselineDiff: diff,
		Sessions:     map[string]string{},
		Stages:       map[string]string{},
		Paths:        map[string]string{},
		Workflow:     workflow,
	}
	if e.PlanningSessionID != "" {
		tool := e.PlanningTool
		if tool == "" {
			tool = "codex"
		}
		state.Sessions[sessionStateKey(tool, "planner")] = e.PlanningSessionID
	}
	if err := snapshotRunPrompts(e.Repo, state.RunID); err != nil {
		return State{}, err
	}
	if err := snapshotRunAcceptance(e.Repo, state.RunID, acceptanceSource); err != nil {
		return State{}, err
	}
	if err := saveState(e.Repo, state); err != nil {
		return State{}, err
	}
	return state, nil
}

// run advances stages until the workflow is done or aborted.
func (e *Engine) run(ctx context.Context, state State) error {
	if !hasWorkflowConfig(state) {
		return fmt.Errorf("run %s 缺少 workflow_config 快照", state.RunID)
	}
	if state.Engine != "go-dag" {
		unlock, err := acquireLock(e.Repo, state.RunID)
		if err != nil {
			return err
		}
		defer unlock()
		return e.runLoop(ctx, state)
	}
	return e.runGoDAG(ctx, state)
}

// runLoop advances stages while the caller holds the run lock.
func (e *Engine) runLoop(ctx context.Context, state State) error {
	e.printProgress(state, "resuming")
	for state.Status == statusRunning {
		forceRun := shouldForceStageRerun(state)
		done := false
		var err error
		if !forceRun {
			done, err = e.artifactDone(state)
			if err != nil {
				gateErr := e.stageArtifactGateError(state, err)
				if handled, handleErr := e.handleStageArtifactGateFailure(&state, gateErr); handleErr != nil {
					return handleErr
				} else if handled {
					continue
				}
				return gateErr
			}
		}
		if !done || forceRun {
			if err := e.detectManualIntervention(&state); err != nil {
				return err
			}
			if err := e.runStage(ctx, &state); err != nil {
				return err
			}
		} else {
			state.Stages[state.Stage] = "completed"
			e.printProgress(state, "skipped")
		}
		done, err = e.checkStageArtifactGate(state)
		if err != nil {
			if handled, handleErr := e.handleStageArtifactGateFailure(&state, err); handleErr != nil {
				return handleErr
			} else if handled {
				continue
			}
			return err
		}
		if !done {
			continue
		}
		clearStageArtifactGateFailure(&state)
		if state.Stage == "execution" {
			preflightPassed, err := e.runAcceptancePreflight(&state)
			if err != nil {
				return err
			}
			if !preflightPassed {
				if err := saveState(e.Repo, state); err != nil {
					return err
				}
				e.printProgress(state, "blocked")
				continue
			}
		}
		validationPassed, err := e.validateStage(ctx, &state)
		if err != nil {
			return err
		}
		if !validationPassed {
			if err := saveState(e.Repo, state); err != nil {
				return err
			}
			if state.Status == statusValidationBlocked {
				e.printProgress(state, "blocked")
			} else {
				e.printProgress(state, "validation_failed")
			}
			continue
		}
		if state.Status != statusRunning {
			if err := saveState(e.Repo, state); err != nil {
				return err
			}
			e.printProgress(state, "blocked")
			continue
		}
		if err := e.advance(&state); err != nil {
			if handled, handleErr := e.handleStageArtifactGateFailure(&state, err); handleErr != nil {
				return handleErr
			} else if handled {
				continue
			}
			return err
		}
		if err := saveState(e.Repo, state); err != nil {
			return err
		}
		e.printProgress(state, "next")
	}
	return nil
}

// handleStageArtifactGateFailure records retriable artifact failures before the workflow fails.
func (e *Engine) handleStageArtifactGateFailure(state *State, failure error) (bool, error) {
	if !isStageArtifactGateError(failure) {
		return false, nil
	}
	if err := recordStageArtifactGateFailure(e.Repo, state, failure); err != nil {
		return true, err
	}
	if err := saveState(e.Repo, *state); err != nil {
		return true, err
	}
	if state.Status == statusValidationBlocked {
		e.printProgress(*state, "blocked")
	} else {
		e.printProgress(*state, "validation_failed")
	}
	return true, nil
}
