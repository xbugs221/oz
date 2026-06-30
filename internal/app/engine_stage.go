// Package app contains workflow engine state and execution boundaries.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

// runStage builds the stage prompt and invokes the proper agent session.
func (e *Engine) runStage(ctx context.Context, state *State) error {
	if state.Sessions == nil {
		state.Sessions = map[string]string{}
	}
	role := stageSessionRole(state.Stage)
	if state.Paths == nil {
		state.Paths = map[string]string{}
	}
	if state.Stages == nil {
		state.Stages = map[string]string{}
	}
	if state.StageTimings == nil {
		state.StageTimings = map[string]StageTiming{}
	}
	prompt, err := promptForStage(e.Repo, *state)
	if err != nil {
		return err
	}
	options, err := e.stageOptionsForRun(state)
	if err != nil {
		return err
	}
	tool, err := e.Registry.Tool(options.Tool)
	if err != nil {
		return err
	}
	runner := tool.NewRunner()
	if e.stageRuntime == nil {
		e.stageRuntime = map[string]stageRuntime{}
	}
	e.stageRuntime[state.Stage] = stageRuntime{}
	timing := state.StageTimings[state.Stage]
	timing.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	timing.FinishedAt = ""
	state.StageTimings[state.Stage] = timing
	state.Stages[state.Stage] = "running"
	if err := saveState(e.Repo, *state); err != nil {
		return err
	}
	e.printProgress(*state, "running")
	sessionKey := sessionStateKey(options.Tool, role)
	if runner, ok := runner.(progressSetter); ok {
		runner.SetProgress(&stageProgressWriter{engine: e, state: state, sessionKey: sessionKey})
	}
	sessionID, err := runner.Run(ctx, e.Repo, prompt, state.Sessions[sessionKey], options)
	if err != nil {
		timing := state.StageTimings[state.Stage]
		timing.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		state.StageTimings[state.Stage] = timing
		if ctx.Err() != nil {
			state.Stages[state.Stage] = statusInterrupted
			saveErr := saveState(e.Repo, *state)
			warnWorkflowWrite("save interrupted stage state", saveErr)
			return errors.Join(err, saveErr)
		} else {
			state.Stages[state.Stage] = statusFailed
			saveErr := saveState(e.Repo, *state)
			warnWorkflowWrite("save failed stage state", saveErr)
			return errors.Join(err, saveErr)
		}
	}
	if sessionID != "" {
		state.Sessions[sessionKey] = sessionID
		meta := e.stageRuntime[state.Stage]
		if meta.Thread == "" {
			meta.Thread = sessionID
			e.stageRuntime[state.Stage] = meta
		}
	}
	timing = state.StageTimings[state.Stage]
	timing.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	state.StageTimings[state.Stage] = timing
	head, diff, snapshotErr := gitSnapshot(e.Repo)
	if snapshotErr != nil {
		return snapshotErr
	}
	state.BaselineHead = head
	state.BaselineDiff = diff
	return saveState(e.Repo, *state)
}

func warnWorkflowWrite(action string, err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "oz flow warning: %s: %v\n", action, err)
}

// stageOptionsForRun resolves dynamic stage options and persists automatic escalations.
func (e *Engine) stageOptionsForRun(state *State) (StageOptions, error) {
	options, err := state.Workflow.StageOption(state.Stage)
	if err != nil {
		return StageOptions{}, err
	}
	escalation, err := fixEscalation(e.Repo, *state)
	if err != nil {
		return StageOptions{}, err
	}
	if !escalation.Enabled {
		return options, nil
	}
	options.Reasoning = higherReasoning(options.Reasoning, escalation.Reasoning)
	options.Fast = false
	state.Workflow.Stages[state.Stage] = options
	if err := saveState(e.Repo, *state); err != nil {
		return StageOptions{}, err
	}
	return options, nil
}

// validateStage runs configured deterministic checks before a stage may advance.
func (e *Engine) validateStage(ctx context.Context, state *State) (bool, error) {
	ensureWorkflowConfig(state)
	if !shouldValidateStage(*state) {
		return true, nil
	}
	if state.Validation == nil {
		state.Validation = map[string]StageValidationState{}
	}
	current := state.Validation[state.Stage]
	current.Attempts++
	attempt := runStageValidation(ctx, e.Repo, state.ChangeName, state.Stage, current.Attempts, state.Workflow.Validation)
	artifactPath, err := writeValidationAttempt(e.Repo, state.RunID, attempt)
	if err != nil {
		return false, err
	}
	current.Kind = validationAttemptKind(attempt)
	current.LastArtifact = artifactPath
	current.Status = attempt.Status
	current.LastError = firstValidationError(attempt)
	state.Validation[state.Stage] = current
	if attempt.Status == validationStatusPassed {
		clearStageValidationFailure(state)
		return true, nil
	}
	if current.Attempts >= state.Workflow.Validation.MaxAttemptsPerStage {
		state.Status = statusValidationBlocked
		state.Stage = statusValidationBlocked
		state.Error = current.LastError
		return false, nil
	}
	state.Stages[state.Stage] = "validation_failed"
	return false, nil
}
