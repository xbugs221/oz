// Package app contains workflow engine state and execution boundaries.
package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// Resume loads the newest unfinished run and continues from its current stage.
func (e *Engine) Resume(ctx context.Context) error {
	return e.resume(ctx, false)
}

// ResumeAfterUserChoice resumes after the interactive menu made the lock decision explicit.
func (e *Engine) ResumeAfterUserChoice(ctx context.Context) error {
	return e.resume(ctx, true)
}

// ResumeDetachedAfterUserChoice starts an unfinished run in the background after an explicit menu choice.
func (e *Engine) ResumeDetachedAfterUserChoice(ctx context.Context, runID string) error {
	_ = ctx
	state, err := loadState(e.Repo, runID)
	if err != nil {
		return err
	}
	if state.Status == statusBlocked || state.Stage == statusBlocked {
		e.printProgress(state, "blocked")
		return fmt.Errorf("run %s 已到达 blocked_review_limit，无法自动继续", runID)
	}
	if state.Status == statusValidationBlocked || state.Stage == statusValidationBlocked {
		e.printProgress(state, "blocked")
		return fmt.Errorf("run %s 已到达 blocked_validation_limit，无法自动继续", runID)
	}
	status, err := lockFileStatus(e.Repo, runID, runtime.GOOS)
	if err != nil {
		return err
	}
	if status == lockStatusActive {
		return newRunLockedError(runID)
	}
	if status == lockStatusUnknown {
		if err := os.Remove(filepath.Join(runDir(e.Repo, runID), "lock")); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := startDetachedCommand(e.Repo, runID); err != nil {
		return err
	}
	if e.stageRuntime == nil {
		e.stageRuntime = map[string]stageRuntime{}
	}
	e.stageRuntime[state.Stage] = stageRuntime{}
	e.printProgress(state, "submitted")
	return nil
}

// ResumeRunJSON resumes a specific run, emits its runner DTO, then continues the workflow.
func (e *Engine) ResumeRunJSON(ctx context.Context, runID string, stdout io.Writer) error {
	return e.resumeRun(ctx, runID, false, stdout)
}

// resume loads the newest recoverable run and handles lock policy before continuing.
func (e *Engine) resume(ctx context.Context, allowUnknownLock bool) error {
	runID, err := FindUnfinishedRun(e.Repo)
	if err != nil {
		return err
	}
	if runID == "" {
		return fmt.Errorf("没有未完成 run")
	}
	return e.resumeRun(ctx, runID, allowUnknownLock, nil)
}

// resumeRun loads a specific recoverable run and handles lock policy before continuing.
func (e *Engine) resumeRun(ctx context.Context, runID string, allowUnknownLock bool, startupJSON io.Writer) error {
	state, err := loadState(e.Repo, runID)
	if err != nil {
		return err
	}
	if state.Status == statusBlocked || state.Stage == statusBlocked {
		e.printProgress(state, "blocked")
		return fmt.Errorf("run %s 已到达 blocked_review_limit，无法自动继续", runID)
	}
	if state.Status == statusValidationBlocked || state.Stage == statusValidationBlocked {
		e.printProgress(state, "blocked")
		return fmt.Errorf("run %s 已到达 blocked_validation_limit，无法自动继续", runID)
	}
	status, err := lockFileStatus(e.Repo, runID, runtime.GOOS)
	if err != nil {
		return err
	}
	if status == lockStatusActive {
		return newRunLockedError(runID)
	}
	if status == lockStatusUnknown {
		if !allowUnknownLock {
			return fmt.Errorf("run %s 存在无法确认的 lock，请通过交互菜单恢复或中止", runID)
		}
		if err := os.Remove(filepath.Join(runDir(e.Repo, runID), "lock")); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	unlock, err := acquireLock(e.Repo, runID)
	if err != nil {
		return err
	}
	defer unlock()
	if !hasWorkflowConfig(state) {
		return fmt.Errorf("run %s 缺少 workflow_config 快照", runID)
	}
	normalizeWorkflowConfig(&state.Workflow)
	if err := e.Registry.ResolveForWorkflow(state.Workflow); err != nil {
		return err
	}
	if err := saveState(e.Repo, state); err != nil {
		return err
	}
	if startupJSON != nil {
		if err := writeRunnerState(startupJSON, state); err != nil {
			return err
		}
		flushWriter(startupJSON)
	}
	if state.Engine == "go-dag" {
		return e.runGoDAGLocked(ctx, state)
	}
	return e.runLoop(ctx, state)
}
