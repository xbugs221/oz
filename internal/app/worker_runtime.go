// Package app records durable worker runtime diagnostics for detached oz flow jobs.
package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"time"
)

const (
	workerExitCompleted       = "completed"
	workerExitError           = "error"
	workerExitPanic           = "panic"
	workerHeartbeatInterval   = 15 * time.Second
	workerHeartbeatStaleAfter = 5 * time.Minute
)

var workerDiagnosticStderr io.Writer = os.Stderr

// startDetachedWorkerCommand starts cmd with stdout and stderr appended to logPath.
func startDetachedWorkerCommand(cmd *exec.Cmd, logPath string) error {
	logFile, err := attachDetachedWorkerLog(cmd, logPath)
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return err
	}
	fmt.Fprintf(logFile, "[%s] spawned pid=%d\n", time.Now().UTC().Format(time.RFC3339Nano), cmd.Process.Pid)
	_ = logFile.Close()
	return cmd.Process.Release()
}

// attachDetachedWorkerLog connects both worker output streams to one persistent log file.
func attachDetachedWorkerLog(cmd *exec.Cmd, logPath string) (*os.File, error) {
	if logPath == "" {
		return nil, fmt.Errorf("worker log path 不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(logFile, "\n[%s] launching %q\n", time.Now().UTC().Format(time.RFC3339Nano), cmd.Args)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	return logFile, nil
}

// runWorkerLogPath returns the diagnostic log path owned by one workflow run.
func runWorkerLogPath(repo, runID string) string {
	return filepath.Join(runDir(repo, runID), "worker.log")
}

// batchWorkerLogPath returns the diagnostic log path owned by one batch worker.
func batchWorkerLogPath(repo, batchID string) string {
	return filepath.Join(batchDir(repo, batchID), "worker.log")
}

// loopWorkerLogPath returns the diagnostic log path owned by the repository loop monitor.
func loopWorkerLogPath(repo string) (string, error) {
	base, err := repoRuntimeDir(repo)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "loop-worker.log"), nil
}

// withRunWorkerLifecycle records start, heartbeat, and terminal exit for one locked run.
func withRunWorkerLifecycle(repo string, state State, fn func() error) (err error) {
	runID := state.RunID
	if err := recordRunWorkerStart(repo, runID, runLifecycleLogPath(repo, state), time.Now()); err != nil {
		return err
	}
	stop := startWorkerHeartbeat(func(now time.Time) error {
		return recordRunWorkerHeartbeat(repo, runID, now)
	})
	defer func() {
		stop()
		if recovered := recover(); recovered != nil {
			err = workerPanicError(recovered)
			writeWorkerPanic(recovered)
			warnWorkflowWrite("record run worker panic", recordRunWorkerExit(repo, runID, workerExitPanic, err.Error(), time.Now()))
			return
		}
		if err != nil {
			warnWorkflowWrite("record run worker error", recordRunWorkerExit(repo, runID, workerExitError, err.Error(), time.Now()))
			return
		}
		warnWorkflowWrite("record run worker completion", recordRunWorkerExit(repo, runID, workerExitCompleted, "", time.Now()))
	}()
	return fn()
}

// runLifecycleLogPath returns the actual worker log that owns this run.
func runLifecycleLogPath(repo string, state State) string {
	if state.BatchID != "" {
		return batchWorkerLogPath(repo, state.BatchID)
	}
	return runWorkerLogPath(repo, state.RunID)
}

// withBatchWorkerLifecycle records start, heartbeat, and terminal exit for one batch worker.
func withBatchWorkerLifecycle(repo, batchID string, fn func() error) (err error) {
	if err := recordBatchWorkerStart(repo, batchID, batchWorkerLogPath(repo, batchID), time.Now()); err != nil {
		return err
	}
	stop := startWorkerHeartbeat(func(now time.Time) error {
		return recordBatchWorkerHeartbeat(repo, batchID, now)
	})
	defer func() {
		stop()
		if recovered := recover(); recovered != nil {
			err = workerPanicError(recovered)
			writeWorkerPanic(recovered)
			warnWorkflowWrite("record batch worker panic", recordBatchWorkerExit(repo, batchID, workerExitPanic, err.Error(), time.Now()))
			return
		}
		if err != nil {
			warnWorkflowWrite("record batch worker error", recordBatchWorkerExit(repo, batchID, workerExitError, err.Error(), time.Now()))
			return
		}
		warnWorkflowWrite("record batch worker completion", recordBatchWorkerExit(repo, batchID, workerExitCompleted, "", time.Now()))
	}()
	return fn()
}

// withLoopWorkerLifecycle records start, heartbeat, and terminal exit for the repo monitor.
func withLoopWorkerLifecycle(repo string, fn func() error) (err error) {
	logPath, err := loopWorkerLogPath(repo)
	if err != nil {
		return err
	}
	if err := recordLoopWorkerStart(repo, logPath, time.Now()); err != nil {
		return err
	}
	stop := startWorkerHeartbeat(func(now time.Time) error {
		return recordLoopWorkerHeartbeat(repo, now)
	})
	defer func() {
		stop()
		if recovered := recover(); recovered != nil {
			err = workerPanicError(recovered)
			writeWorkerPanic(recovered)
			warnWorkflowWrite("record loop worker panic", recordLoopWorkerExit(repo, workerExitPanic, err.Error(), time.Now()))
			return
		}
		if err != nil {
			warnWorkflowWrite("record loop worker error", recordLoopWorkerExit(repo, workerExitError, err.Error(), time.Now()))
			return
		}
		warnWorkflowWrite("record loop worker completion", recordLoopWorkerExit(repo, workerExitCompleted, "", time.Now()))
	}()
	return fn()
}

// startWorkerHeartbeat periodically persists liveness while a worker is still running.
func startWorkerHeartbeat(beat func(time.Time) error) func() {
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(workerHeartbeatInterval)
		defer ticker.Stop()
		defer close(done)
		for {
			select {
			case <-ticker.C:
				warnWorkflowWrite("record worker heartbeat", beat(time.Now()))
			case <-stop:
				return
			}
		}
	}()
	return func() {
		close(stop)
		<-done
	}
}

// recordRunWorkerStart stores the current process identity in a run state file.
func recordRunWorkerStart(repo, runID, logPath string, now time.Time) error {
	return mergeState(repo, runID, func(state *State) {
		state.Worker = newWorkerRuntime(logPath, now)
	})
}

// recordRunWorkerHeartbeat refreshes one run worker heartbeat timestamp.
func recordRunWorkerHeartbeat(repo, runID string, now time.Time) error {
	return mergeState(repo, runID, func(state *State) {
		ensureWorkerRuntime(&state.Worker, "", now)
		state.Worker.LastHeartbeatAt = timestampUTC(now)
	})
}

// recordRunWorkerExit stores the terminal worker exit without overwriting completed workflows.
func recordRunWorkerExit(repo, runID, exit, message string, now time.Time) error {
	return mergeState(repo, runID, func(state *State) {
		markWorkerRuntimeExit(&state.Worker, exit, message, now)
		if exit == workerExitPanic && state.Status == statusRunning {
			state.Status = statusFailed
			if state.Error == "" {
				state.Error = message
			}
		}
	})
}

// recordBatchWorkerStart stores the current process identity in a batch state file.
func recordBatchWorkerStart(repo, batchID, logPath string, now time.Time) error {
	return withBatchState(repo, batchID, func(batch *BatchState) error {
		batch.Worker = newWorkerRuntime(logPath, now)
		return nil
	})
}

// recordBatchWorkerHeartbeat refreshes one batch worker heartbeat timestamp.
func recordBatchWorkerHeartbeat(repo, batchID string, now time.Time) error {
	return withBatchState(repo, batchID, func(batch *BatchState) error {
		ensureWorkerRuntime(&batch.Worker, "", now)
		batch.Worker.LastHeartbeatAt = timestampUTC(now)
		return nil
	})
}

// recordBatchWorkerExit stores the terminal batch worker exit reason.
func recordBatchWorkerExit(repo, batchID, exit, message string, now time.Time) error {
	return withBatchState(repo, batchID, func(batch *BatchState) error {
		markWorkerRuntimeExit(&batch.Worker, exit, message, now)
		if exit == workerExitPanic && batch.Status == batchStatusRunning {
			batch.Status = batchStatusFailed
			if batch.Error == "" {
				batch.Error = message
			}
		}
		return nil
	})
}

// recordLoopWorkerStart writes the monitor process identity into repo runtime state.
func recordLoopWorkerStart(repo, logPath string, now time.Time) error {
	return writeLoopWorkerState(repo, *newWorkerRuntime(logPath, now))
}

// recordLoopWorkerHeartbeat refreshes the monitor heartbeat JSON.
func recordLoopWorkerHeartbeat(repo string, now time.Time) error {
	state, err := loadLoopWorkerState(repo)
	if err != nil {
		return err
	}
	state.LastHeartbeatAt = timestampUTC(now)
	return writeLoopWorkerState(repo, state)
}

// recordLoopWorkerExit stores the monitor terminal exit reason.
func recordLoopWorkerExit(repo, exit, message string, now time.Time) error {
	state, err := loadLoopWorkerState(repo)
	if err != nil {
		return err
	}
	worker := &state
	markWorkerRuntimeExit(&worker, exit, message, now)
	return writeLoopWorkerState(repo, *worker)
}

// loadLoopWorkerState reads the repo monitor diagnostic state file.
func loadLoopWorkerState(repo string) (WorkerRuntimeState, error) {
	path, err := loopWorkerStatePath(repo)
	if err != nil {
		return WorkerRuntimeState{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkerRuntimeState{}, err
	}
	var state WorkerRuntimeState
	if err := json.Unmarshal(data, &state); err != nil {
		return WorkerRuntimeState{}, err
	}
	return state, nil
}

// writeLoopWorkerState writes the repo monitor diagnostic state file.
func writeLoopWorkerState(repo string, state WorkerRuntimeState) error {
	path, err := loopWorkerStatePath(repo)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// loopWorkerStatePath returns the repo monitor diagnostic JSON path.
func loopWorkerStatePath(repo string) (string, error) {
	base, err := repoRuntimeDir(repo)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "loop-worker.json"), nil
}

// newWorkerRuntime captures current process metadata for durable diagnostics.
func newWorkerRuntime(logPath string, now time.Time) *WorkerRuntimeState {
	hostname, _ := os.Hostname()
	stamp := timestampUTC(now)
	return &WorkerRuntimeState{
		PID:             os.Getpid(),
		Hostname:        hostname,
		StartedAt:       stamp,
		LastHeartbeatAt: stamp,
		LogPath:         logPath,
	}
}

// ensureWorkerRuntime allocates worker state for legacy records that lack it.
func ensureWorkerRuntime(target **WorkerRuntimeState, logPath string, now time.Time) {
	if *target == nil {
		*target = newWorkerRuntime(logPath, now)
	}
}

// markWorkerRuntimeExit finalizes an existing worker state record.
func markWorkerRuntimeExit(target **WorkerRuntimeState, exit, message string, now time.Time) {
	ensureWorkerRuntime(target, "", now)
	stamp := timestampUTC(now)
	(*target).LastHeartbeatAt = stamp
	(*target).FinishedAt = stamp
	(*target).Exit = exit
	(*target).Error = message
}

// workerHeartbeatExpired reports live state whose worker has stopped heartbeating.
func workerHeartbeatExpired(worker *WorkerRuntimeState, now time.Time) bool {
	if worker == nil || worker.Exit != "" || worker.LastHeartbeatAt == "" {
		return false
	}
	last, err := time.Parse(time.RFC3339Nano, worker.LastHeartbeatAt)
	if err != nil {
		return false
	}
	return now.Sub(last) > workerHeartbeatStaleAfter
}

// workerPanicError converts recovered panic values into a normal workflow error.
func workerPanicError(recovered any) error {
	return fmt.Errorf("worker panic: %v", recovered)
}

// writeWorkerPanic writes a panic stack into stderr, which detached workers persist in worker.log.
func writeWorkerPanic(recovered any) {
	fmt.Fprintf(workerDiagnosticStderr, "oz flow worker panic: %v\n%s\n", recovered, debug.Stack())
}

// timestampUTC formats worker timestamps consistently across state files.
func timestampUTC(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
