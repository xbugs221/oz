// Package app implements explicit restart of stopped sealed workflows and batches.
package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// restartHuman resolves oz flow restart's human aliases and starts the selected worker detached.
func restartHuman(ctx context.Context, stdout io.Writer, repo string, engine *Engine, args []string) error {
	_ = ctx
	kind, ref, err := ResolveRestartTarget(repo, args)
	if err != nil {
		return err
	}
	switch kind {
	case "batch":
		if err := engine.RestartBatchDetached(ref.ID); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "已重启批量任务 %s\n", ref.Alias)
		return nil
	case "run":
		if err := engine.RestartRunDetached(ref.ID, true); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "已重启工作流 %s\n", ref.Alias)
		return nil
	default:
		return fmt.Errorf("没有可重启任务")
	}
}

// ResolveRestartTarget maps restart arguments to a restartable batch or run.
func ResolveRestartTarget(repo string, args []string) (kind string, ref StatusRef, err error) {
	if len(args) == 0 {
		batches, err := ListBatchRefs(repo)
		if err != nil {
			return "", StatusRef{}, err
		}
		for _, batchRef := range batches {
			batch, err := loadBatchState(repo, batchRef.ID)
			if err == nil && isRestartableBatch(repo, batch) {
				return "batch", batchRef, nil
			}
		}
		runs, err := ListRunRefs(repo)
		if err != nil {
			return "", StatusRef{}, err
		}
		for _, runRef := range runs {
			state, err := loadState(repo, runRef.ID)
			if err == nil && state.BatchID == "" && isRestartableRunCandidate(repo, state) {
				return "run", runRef, nil
			}
		}
		return "", StatusRef{}, fmt.Errorf("没有可重启任务")
	}
	if len(args) != 1 {
		return "", StatusRef{}, fmt.Errorf("用法：oz flow restart [-bN|-wN]")
	}
	switch {
	case strings.HasPrefix(args[0], "-b"):
		ref, err := resolveIndexedRef(repo, args[0], "-b", ListBatchRefs)
		return "batch", ref, err
	case strings.HasPrefix(args[0], "-w"):
		ref, err := resolveIndexedRef(repo, args[0], "-w", ListRunRefs)
		return "run", ref, err
	default:
		return "", StatusRef{}, fmt.Errorf("用法：oz flow restart [-bN|-wN]")
	}
}

// RestartRunDetached resets a recoverable run and starts a detached worker.
func (e *Engine) RestartRunDetached(runID string, allowUnknownLock bool) error {
	if _, err := e.prepareRestartRun(runID, allowUnknownLock); err != nil {
		return err
	}
	return startDetachedCommand(e.Repo, runID)
}

// RestartRunJSON resets a recoverable run, emits its DTO, then continues inline.
func (e *Engine) RestartRunJSON(ctx context.Context, runID string, stdout io.Writer) error {
	if _, err := e.prepareRestartRun(runID, false); err != nil {
		_ = writeFailedRunnerError(stdout, runID, err)
		return err
	}
	if err := e.ResumeRunJSON(ctx, runID, stdout); err != nil {
		if state, loadErr := loadState(e.Repo, runID); loadErr == nil {
			if isRunLockedError(err) {
				state.Error = err.Error()
				_ = writeRunnerState(stdout, state)
				return err
			}
			state = failedState(state, err)
			_ = saveState(e.Repo, state)
			_ = writeFailedRunnerState(stdout, state, err)
		} else {
			_ = writeFailedRunnerError(stdout, runID, err)
		}
		return err
	}
	return nil
}

// prepareRestartRun validates and rewrites only resumable run state.
func (e *Engine) prepareRestartRun(runID string, allowUnknownLock bool) (State, error) {
	state, err := loadState(e.Repo, runID)
	if err != nil {
		return State{}, err
	}
	if err := ensureRunRestartable(state); err != nil {
		return State{}, err
	}
	if err := clearRestartLock(e.Repo, runID, allowUnknownLock); err != nil {
		return State{}, err
	}
	state.Status = statusRunning
	state.Error = ""
	if state.Stage == "" || state.Stage == statusInterrupted || state.Stage == statusAcceptanceContractBlocked {
		state.Stage = "execution"
	}
	if state.Stages != nil && state.Stage != "" && state.Stages[state.Stage] == statusInterrupted {
		delete(state.Stages, state.Stage)
	}
	if !hasWorkflowConfig(state) {
		return State{}, fmt.Errorf("run %s 缺少 workflow_config 快照", runID)
	}
	normalizeWorkflowConfig(&state.Workflow)
	if err := e.Registry.ResolveForWorkflow(state.Workflow); err != nil {
		return State{}, err
	}
	if err := saveState(e.Repo, state); err != nil {
		return State{}, err
	}
	return state, nil
}

// RestartBatchDetached resets a recoverable batch and starts a detached batch worker.
func (e *Engine) RestartBatchDetached(batchID string) error {
	if err := e.prepareRestartBatch(batchID, true); err != nil {
		return err
	}
	return startDetachedBatchCommand(e.Repo, batchID)
}

// RestartBatchJSON resets a recoverable batch and continues it inline.
func (e *Engine) RestartBatchJSON(ctx context.Context, batchID string) error {
	if err := e.prepareRestartBatch(batchID, false); err != nil {
		return err
	}
	return e.RunBatch(ctx, batchID)
}

// prepareRestartBatch validates the current queue item before restoring batch state.
func (e *Engine) prepareRestartBatch(batchID string, allowUnknownLock bool) error {
	batch, err := loadBatchState(e.Repo, batchID)
	if err != nil {
		return err
	}
	switch batch.Status {
	case batchStatusRunning:
	case batchStatusFailed:
	default:
		return fmt.Errorf("批量任务 %s 已达到 %s，不能自动重启", batchID, batch.Status)
	}
	if batch.CurrentIndex < len(batch.Changes) {
		changeName := batch.Changes[batch.CurrentIndex]
		runID := batch.RunIDs[changeName]
		if runID == "" {
			runID = batch.FailedRunID
		}
		if runID != "" {
			if state, err := loadState(e.Repo, runID); err == nil && batch.Status == batchStatusRunning && state.Status == statusDone {
				return nil
			}
			if batch.Status == batchStatusFailed {
				// For failed batches: check lock and run state, then clear association
				// so batch worker creates a fresh run for the current change.
				state, err := loadState(e.Repo, runID)
				if err != nil {
					prefix := ""
					if changeName != "" {
						prefix = changeName + " 的"
					}
					return fmt.Errorf("%s工作流记录缺失，无法自动确认恢复方式", prefix)
				}
				if err := ensureRunRestartable(state); err != nil {
					return err
				}
				if err := clearRestartLock(e.Repo, runID, allowUnknownLock); err != nil {
					return err
				}
			} else {
				if _, err := e.prepareRestartRun(runID, allowUnknownLock); err != nil {
					return err
				}
			}
		}
	}
	return withBatchState(e.Repo, batchID, func(b *BatchState) error {
		b.Status = batchStatusRunning
		b.FailedChange = ""
		b.FailedRunID = ""
		b.Error = ""
		// For failed batches: clear the current change's run association so
		// the batch worker creates a fresh run for this change.
		if b.CurrentIndex < len(b.Changes) && batch.Status == batchStatusFailed {
			delete(b.RunIDs, b.Changes[b.CurrentIndex])
		}
		return nil
	})
}

// ensureRunRestartable rejects terminal states that require manual intervention.
func ensureRunRestartable(state State) error {
	switch state.Status {
	case statusFailed, statusInterrupted, statusRunning, statusAcceptanceContractBlocked:
		return nil
	case statusDone:
		return fmt.Errorf("工作流 %s 已完成，不能重启", state.RunID)
	case statusBlocked, statusValidationBlocked, statusAborted, "aborted":
		return fmt.Errorf("工作流 %s 已达到 %s，不能自动重启", state.RunID, state.Status)
	default:
		return fmt.Errorf("工作流 %s 状态 %s 不能重启", state.RunID, state.Status)
	}
}

func isRestartableRunState(state State) bool {
	return ensureRunRestartable(state) == nil
}

// isRestartableRunCandidate excludes live work from implicit restart selection.
func isRestartableRunCandidate(repo string, state State) bool {
	if !isRestartableRunState(state) {
		return false
	}
	active, err := restartLockActive(repo, state.RunID)
	return err == nil && !active
}

// isRestartableBatch reports whether a batch can be chosen by default restart.
func isRestartableBatch(repo string, batch BatchState) bool {
	runID := restartBatchCurrentRunID(batch)
	if batch.Status == batchStatusRunning {
		if runID == "" {
			return true
		}
		active, err := restartLockActive(repo, runID)
		return err == nil && !active
	}
	if batch.Status != batchStatusFailed {
		return false
	}
	if batch.CurrentIndex >= len(batch.Changes) {
		return true
	}
	if runID == "" {
		return true
	}
	state, err := loadState(repo, runID)
	return err == nil && isRestartableRunCandidate(repo, state)
}

// restartBatchCurrentRunID returns the current durable run attached to a batch.
func restartBatchCurrentRunID(batch BatchState) string {
	if batch.CurrentIndex >= len(batch.Changes) {
		return ""
	}
	runID := batch.RunIDs[batch.Changes[batch.CurrentIndex]]
	if runID == "" {
		runID = batch.FailedRunID
	}
	return runID
}

// restartLockActive classifies only active locks as live work for default selection.
func restartLockActive(repo, runID string) (bool, error) {
	status, err := lockFileStatus(repo, runID, runtime.GOOS)
	if err != nil {
		return false, err
	}
	return status == lockStatusActive, nil
}

// clearRestartLock removes only stale or explicitly allowed unknown locks.
func clearRestartLock(repo, runID string, allowUnknownLock bool) error {
	status, err := lockFileStatus(repo, runID, runtime.GOOS)
	if err != nil {
		return err
	}
	switch status {
	case lockStatusActive:
		return newRunLockedError(runID)
	case lockStatusUnknown:
		if !allowUnknownLock {
			return fmt.Errorf("run %s 存在无法确认的 lock，请通过交互菜单恢复或中止", runID)
		}
	}
	if status == lockStatusStale || status == lockStatusUnknown {
		if err := os.Remove(filepath.Join(runDir(repo, runID), "lock")); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
