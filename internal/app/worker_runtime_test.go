// Package app tests durable diagnostics for oz flow worker processes.
package app

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestAttachDetachedWorkerLogPersistsOutput verifies detached workers have a durable log sink.
func TestAttachDetachedWorkerLogPersistsOutput(t *testing.T) {
	cmd := exec.Command("oz", "flow", "resume", "--run-id", "run-1", "--json")
	logPath := filepath.Join(t.TempDir(), "worker.log")
	logFile, err := attachDetachedWorkerLog(cmd, logPath)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Stdout != logFile || cmd.Stderr != logFile {
		t.Fatal("worker stdout/stderr must both point at worker.log")
	}
	if err := logFile.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "launching") {
		t.Fatalf("worker log missing launch header: %s", data)
	}
}

// TestRunWorkerLifecycleRecordsPanic verifies panics become durable run diagnostics.
func TestRunWorkerLifecycleRecordsPanic(t *testing.T) {
	repo := gitRepo(t)
	runID := "worker-panic-run"
	if err := saveState(repo, workerRuntimeTestState(runID)); err != nil {
		t.Fatal(err)
	}
	var panicLog bytes.Buffer
	previous := workerDiagnosticStderr
	workerDiagnosticStderr = &panicLog
	t.Cleanup(func() { workerDiagnosticStderr = previous })

	err := withRunWorkerLifecycle(repo, workerRuntimeTestState(runID), func() error {
		panic("boom")
	})
	if err == nil || !strings.Contains(err.Error(), "worker panic: boom") {
		t.Fatalf("panic error = %v, want worker panic", err)
	}
	if !strings.Contains(panicLog.String(), "oz flow worker panic: boom") {
		t.Fatalf("panic log missing stack header: %s", panicLog.String())
	}
	state, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != statusFailed || !strings.Contains(state.Error, "worker panic: boom") {
		t.Fatalf("state status/error = %s/%q, want failed panic", state.Status, state.Error)
	}
	if state.Worker == nil || state.Worker.Exit != workerExitPanic || state.Worker.FinishedAt == "" {
		t.Fatalf("worker state = %#v, want panic with finished_at", state.Worker)
	}
	if state.Worker.LogPath != runWorkerLogPath(repo, runID) {
		t.Fatalf("worker log path = %q, want %q", state.Worker.LogPath, runWorkerLogPath(repo, runID))
	}
}

// TestBatchWorkerLifecycleRecordsErrorWithoutFailingBatch keeps ordinary errors as diagnostics.
func TestBatchWorkerLifecycleRecordsErrorWithoutFailingBatch(t *testing.T) {
	repo := gitRepo(t)
	batchID := "batch-worker-error"
	if err := saveBatchState(repo, BatchState{BatchID: batchID, Status: batchStatusRunning, Changes: []string{"1-demo"}, RunIDs: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("temporary worker error")
	err := withBatchWorkerLifecycle(repo, batchID, func() error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("batch worker error = %v, want %v", err, wantErr)
	}
	batch, err := loadBatchState(repo, batchID)
	if err != nil {
		t.Fatal(err)
	}
	if batch.Status != batchStatusRunning {
		t.Fatalf("batch status = %s, want running", batch.Status)
	}
	if batch.Worker == nil || batch.Worker.Exit != workerExitError || batch.Worker.Error != wantErr.Error() {
		t.Fatalf("batch worker state = %#v, want error diagnostic", batch.Worker)
	}
}

// TestRunWorkerLifecycleUsesBatchLogPath points batch-owned runs at the real batch worker log.
func TestRunWorkerLifecycleUsesBatchLogPath(t *testing.T) {
	repo := gitRepo(t)
	runID := "batch-owned-run"
	batchID := "owning-batch"
	state := workerRuntimeTestState(runID)
	state.BatchID = batchID
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	if err := withRunWorkerLifecycle(repo, state, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	persisted, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Worker == nil || persisted.Worker.LogPath != batchWorkerLogPath(repo, batchID) {
		t.Fatalf("worker log path = %#v, want batch worker log %q", persisted.Worker, batchWorkerLogPath(repo, batchID))
	}
}

// TestRunningRunCannotAdvanceOnExpiredHeartbeat catches live-PID workers that stopped heartbeating.
func TestRunningRunCannotAdvanceOnExpiredHeartbeat(t *testing.T) {
	repo := gitRepo(t)
	runID := "heartbeat-stale-run"
	stale := time.Now().Add(-workerHeartbeatStaleAfter - time.Minute)
	state := workerRuntimeTestState(runID)
	state.Worker = &WorkerRuntimeState{
		PID:             os.Getpid(),
		StartedAt:       timestampUTC(stale),
		LastHeartbeatAt: timestampUTC(stale),
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	hostname, _ := os.Hostname()
	if err := writeJSONFile(filepath.Join(runDir(repo, runID), "lock"), LockInfo{
		PID:       os.Getpid(),
		Hostname:  hostname,
		RunID:     runID,
		StartedAt: timestampUTC(stale),
	}); err != nil {
		t.Fatal(err)
	}
	stopped, reason := runningRunCannotAdvance(repo, state, time.Now())
	if !stopped || reason != "当前 run worker heartbeat 已失效" {
		t.Fatalf("runningRunCannotAdvance = %v/%q, want heartbeat stale", stopped, reason)
	}
}

// workerRuntimeTestState returns the minimal durable state needed by worker tests.
func workerRuntimeTestState(runID string) State {
	return State{
		RunID:      runID,
		ChangeName: "1-demo",
		Status:     statusRunning,
		Stage:      "execution",
		Sessions:   map[string]string{},
		Stages:     map[string]string{},
		Paths:      map[string]string{},
		Workflow:   DefaultWorkflowConfig(),
	}
}
