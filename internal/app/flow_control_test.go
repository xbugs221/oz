// Package app tests operator-level batch stop, archive, and loop control.
package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// TestStopCommandAbortsRunningBatch verifies stop interrupts the active batch run.
func TestStopCommandAbortsRunningBatch(t *testing.T) {
	repo := gitRepo(t)
	runID := "20260617T010000.000000000Z"
	batchID := "20260617T010001.000000000Z"
	if err := saveState(repo, State{RunID: runID, ChangeName: "1-a", Status: statusRunning, Stage: "execution", Sessions: map[string]string{}, Stages: map[string]string{}, Paths: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	if err := saveBatchState(repo, BatchState{
		BatchID:      batchID,
		Status:       batchStatusRunning,
		Changes:      []string{"1-a"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{"1-a": runID},
	}); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := dispatchStopCommand([]string{"stop"}, &stdout, repo); err != nil {
		t.Fatal(err)
	}
	batch, err := loadBatchState(repo, batchID)
	if err != nil {
		t.Fatal(err)
	}
	state, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if batch.Status != batchStatusAborted || state.Status != "aborted" {
		t.Fatalf("stop status = batch %q run %q, want aborted/aborted", batch.Status, state.Status)
	}
	if !strings.Contains(stdout.String(), "已停止批量工作流") {
		t.Fatalf("stop output missing confirmation: %s", stdout.String())
	}
}

// TestArchiveCommandArchivesLatestFailedBatch verifies archive hides a failed batch without deleting evidence.
func TestArchiveCommandArchivesLatestFailedBatch(t *testing.T) {
	repo := gitRepo(t)
	runID := "20260617T020000.000000000Z"
	batchID := "20260617T020001.000000000Z"
	if err := saveState(repo, State{RunID: runID, ChangeName: "2-b", Status: statusFailed, Stage: "execution", Error: "agent failed", Sessions: map[string]string{}, Stages: map[string]string{}, Paths: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	if err := saveBatchState(repo, BatchState{
		BatchID:      batchID,
		Status:       batchStatusFailed,
		Changes:      []string{"2-b"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{"2-b": runID},
		FailedChange: "2-b",
		FailedRunID:  runID,
		Error:        "agent failed",
	}); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := dispatchArchiveCommand([]string{"archive"}, &stdout, repo); err != nil {
		t.Fatal(err)
	}
	batch, err := loadBatchState(repo, batchID)
	if err != nil {
		t.Fatal(err)
	}
	state, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if batch.Status != batchStatusArchived || state.Status != statusArchived {
		t.Fatalf("archive status = batch %q run %q, want archived/%s", batch.Status, state.Status, statusArchived)
	}
	if _, stopped, err := FindStartupBatches(repo); err != nil || len(stopped) != 0 {
		t.Fatalf("archived batch should not appear as startup stopped state, stopped=%d err=%v", len(stopped), err)
	}
	if !strings.Contains(stdout.String(), "已归档失败批量工作流") {
		t.Fatalf("archive output missing confirmation: %s", stdout.String())
	}
}

// TestLoopCommandStartsDetachedWorker verifies human loop returns after submitting the monitor.
func TestLoopCommandStartsDetachedWorker(t *testing.T) {
	repo := gitRepo(t)
	var started []string
	previousStart := startDetachedLoopCommand
	startDetachedLoopCommand = func(repo string) error {
		started = append(started, repo)
		return nil
	}
	t.Cleanup(func() { startDetachedLoopCommand = previousStart })

	var stdout bytes.Buffer
	engine := NewEngine(repo, NewAgentRegistry())
	if err := dispatchLoopCommand(context.Background(), []string{"loop"}, &stdout, repo, engine); err != nil {
		t.Fatal(err)
	}
	if len(started) != 1 || started[0] != repo {
		t.Fatalf("started loop workers = %#v, want repo %s", started, repo)
	}
	if !strings.Contains(stdout.String(), "已启动后台 loop 监控") {
		t.Fatalf("loop output missing detached confirmation: %s", stdout.String())
	}
}

// TestLoopCommandSkipsDuplicateWorker verifies repeated loop commands do not stack monitors.
func TestLoopCommandSkipsDuplicateWorker(t *testing.T) {
	repo := gitRepo(t)
	path, err := loopWorkerLockPath(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	hostname, _ := os.Hostname()
	if err := writeJSONFile(path, LockInfo{PID: os.Getpid(), Hostname: hostname, RunID: "loop", StartedAt: time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
		t.Fatal(err)
	}

	previousStart := startDetachedLoopCommand
	startDetachedLoopCommand = func(repo string) error {
		t.Fatalf("duplicate loop command should not start a new worker for %s", repo)
		return nil
	}
	t.Cleanup(func() { startDetachedLoopCommand = previousStart })

	var stdout bytes.Buffer
	engine := NewEngine(repo, NewAgentRegistry())
	if err := dispatchLoopCommand(context.Background(), []string{"loop"}, &stdout, repo, engine); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "后台 loop 监控已在运行") {
		t.Fatalf("loop output missing duplicate notice: %s", stdout.String())
	}
}

// TestLoopArchivesFailedBatchAndStartsContinuation verifies loop continues from unfinished active changes.
func TestLoopArchivesFailedBatchAndStartsContinuation(t *testing.T) {
	repo := gitRepo(t)
	installFlowControlFakeOz(t)
	for _, name := range []string{"2-b", "3-c"} {
		writeFlowControlChange(t, repo, name)
	}
	runID := "20260617T030000.000000000Z"
	batchID := "20260617T030001.000000000Z"
	if err := saveState(repo, State{RunID: runID, ChangeName: "2-b", Status: statusFailed, Stage: "execution", Error: "backend failed", Sessions: map[string]string{}, Stages: map[string]string{}, Paths: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	if err := saveBatchState(repo, BatchState{
		BatchID:      batchID,
		Status:       batchStatusFailed,
		Changes:      []string{"1-a", "2-b", "3-c"},
		CurrentIndex: 1,
		RunIDs:       map[string]string{"1-a": "done-run", "2-b": runID},
		FailedChange: "2-b",
		FailedRunID:  runID,
		Error:        "backend failed",
	}); err != nil {
		t.Fatal(err)
	}

	var started []string
	previousStart := startDetachedBatchCommand
	startDetachedBatchCommand = func(repo, batchID string) error {
		started = append(started, batchID)
		batch, err := loadBatchState(repo, batchID)
		if err != nil {
			return err
		}
		for _, change := range batch.Changes {
			if err := os.RemoveAll(changePath(repo, change)); err != nil {
				return err
			}
		}
		return withBatchState(repo, batchID, func(batch *BatchState) error {
			batch.Status = batchStatusDone
			batch.CurrentIndex = len(batch.Changes)
			return nil
		})
	}
	t.Cleanup(func() { startDetachedBatchCommand = previousStart })

	var stdout bytes.Buffer
	engine := NewEngine(repo, NewAgentRegistry())
	if err := runBatchLoop(context.Background(), &stdout, repo, engine, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if len(started) != 1 {
		t.Fatalf("started continuation batches = %v, want exactly one", started)
	}
	oldBatch, err := loadBatchState(repo, batchID)
	if err != nil {
		t.Fatal(err)
	}
	oldRun, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	newBatch, err := loadBatchState(repo, started[0])
	if err != nil {
		t.Fatal(err)
	}
	if oldBatch.Status != batchStatusArchived || oldRun.Status != statusArchived {
		t.Fatalf("old status = batch %q run %q, want archived/%s", oldBatch.Status, oldRun.Status, statusArchived)
	}
	if got := strings.Join(newBatch.Changes, ","); got != "2-b,3-c" {
		t.Fatalf("continuation changes = %s, want 2-b,3-c", got)
	}
	if _, reused := newBatch.RunIDs["2-b"]; reused {
		t.Fatalf("continuation batch reused failed run id: %#v", newBatch.RunIDs)
	}
	if !strings.Contains(stdout.String(), "已归档失败批量工作流") || !strings.Contains(stdout.String(), "已完成") {
		t.Fatalf("loop output missing archive/done messages: %s", stdout.String())
	}
}

// TestLoopKeepsRunningBatchOwnedRunWithFreshBatchHeartbeat verifies loop does
// not fail a batch-owned run while the batch worker is still heartbeating.
func TestLoopKeepsRunningBatchOwnedRunWithFreshBatchHeartbeat(t *testing.T) {
	repo := gitRepo(t)
	now := time.Now()
	runID := "20260617T033000.000000000Z"
	batchID := "20260617T033001.000000000Z"
	state := flowControlBatchRunState(runID, batchID, now.Add(-workerHeartbeatStaleAfter-time.Minute))
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	writeFlowControlRunLock(t, repo, runID)
	batch := flowControlBatchState(batchID, "1-a", runID, now)
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	decision := assessBatchForLoop(repo, batch)
	if decision.Status != loopDecisionRunning {
		t.Fatalf("assessBatchForLoop = %#v, want running while batch worker heartbeat is fresh", decision)
	}
}

// TestLoopFailsBatchOwnedRunWhenBatchHeartbeatIsStale verifies stale run
// heartbeat remains actionable when no live batch worker owns the current run.
func TestLoopFailsBatchOwnedRunWhenBatchHeartbeatIsStale(t *testing.T) {
	repo := gitRepo(t)
	now := time.Now()
	stale := now.Add(-workerHeartbeatStaleAfter - time.Minute)
	runID := "20260617T033100.000000000Z"
	batchID := "20260617T033101.000000000Z"
	state := flowControlBatchRunState(runID, batchID, stale)
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	writeFlowControlRunLock(t, repo, runID)
	batch := flowControlBatchState(batchID, "1-a", runID, stale)
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	decision := assessBatchForLoop(repo, batch)
	if decision.Status != loopDecisionFailed || decision.Reason != staleRunWorkerHeartbeatReason {
		t.Fatalf("assessBatchForLoop = %#v, want failed stale heartbeat", decision)
	}
}

// TestLoopStartsActiveChangesAfterCompletedBatch verifies done history does not hide active proposals.
func TestLoopStartsActiveChangesAfterCompletedBatch(t *testing.T) {
	repo := gitRepo(t)
	installFlowControlFakeOz(t)
	for _, name := range []string{"1-a", "2-b"} {
		writeFlowControlChange(t, repo, name)
	}
	if err := saveBatchState(repo, BatchState{
		BatchID:      "20260617T040000.000000000Z",
		Status:       batchStatusDone,
		Changes:      []string{"1-a"},
		CurrentIndex: 1,
		RunIDs:       map[string]string{"1-a": "done-run"},
	}); err != nil {
		t.Fatal(err)
	}

	var started []string
	previousStart := startDetachedBatchCommand
	startDetachedBatchCommand = func(repo, batchID string) error {
		started = append(started, batchID)
		return nil
	}
	t.Cleanup(func() { startDetachedBatchCommand = previousStart })

	var stdout bytes.Buffer
	engine := NewEngine(repo, NewAgentRegistry())
	done, err := runBatchLoopTick(context.Background(), &stdout, repo, engine)
	if err != nil {
		t.Fatal(err)
	}
	if done {
		t.Fatalf("loop tick returned done while active changes still exist")
	}
	if len(started) != 1 {
		t.Fatalf("started batches = %v, want exactly one", started)
	}
	newBatch, err := loadBatchState(repo, started[0])
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(newBatch.Changes, ","); got != "1-a,2-b" {
		t.Fatalf("new batch changes = %s, want 1-a,2-b", got)
	}
	if strings.Contains(stdout.String(), "已完成") {
		t.Fatalf("loop should not print completed while starting active changes: %s", stdout.String())
	}
}

// flowControlBatchRunState creates a running batch-owned run with controlled heartbeat age.
func flowControlBatchRunState(runID, batchID string, heartbeat time.Time) State {
	state := workerRuntimeTestState(runID)
	state.ChangeName = "1-a"
	state.BatchID = batchID
	state.Worker = &WorkerRuntimeState{
		PID:             os.Getpid(),
		StartedAt:       timestampUTC(heartbeat),
		LastHeartbeatAt: timestampUTC(heartbeat),
	}
	return state
}

// flowControlBatchState creates a one-change running batch with controlled worker heartbeat.
func flowControlBatchState(batchID, changeName, runID string, heartbeat time.Time) BatchState {
	return BatchState{
		BatchID:      batchID,
		Status:       batchStatusRunning,
		Changes:      []string{changeName},
		CurrentIndex: 0,
		RunIDs:       map[string]string{changeName: runID},
		Worker: &WorkerRuntimeState{
			PID:             os.Getpid(),
			StartedAt:       timestampUTC(heartbeat),
			LastHeartbeatAt: timestampUTC(heartbeat),
		},
	}
}

// writeFlowControlRunLock records an active lock owned by the test process.
func writeFlowControlRunLock(t *testing.T, repo, runID string) {
	t.Helper()
	hostname, _ := os.Hostname()
	if err := writeJSONFile(filepath.Join(runDir(repo, runID), "lock"), LockInfo{
		PID:       os.Getpid(),
		Hostname:  hostname,
		RunID:     runID,
		StartedAt: timestampUTC(time.Now()),
	}); err != nil {
		t.Fatal(err)
	}
}

// writeFlowControlChange creates a minimal active change directory for fake oz list.
func writeFlowControlChange(t *testing.T, repo, name string) {
	t.Helper()
	dir := filepath.Join(repo, "docs", "changes", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"acceptance.json", "task.md"} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(file+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// installFlowControlFakeOz points ListChanges at this test process.
func installFlowControlFakeOz(t *testing.T) {
	t.Helper()
	previous := ozCommand
	previousPrefix := ozCommandPrefix
	ozCommand = os.Args[0]
	ozCommandPrefix = []string{"-test.run=TestFlowControlFakeOzCommand", "--"}
	t.Setenv("OZ_FLOW_CONTROL_FAKE_OZ", "1")
	t.Cleanup(func() {
		ozCommand = previous
		ozCommandPrefix = previousPrefix
	})
}

// TestFlowControlFakeOzCommand serves the small oz JSON surface used by loop tests.
func TestFlowControlFakeOzCommand(t *testing.T) {
	if os.Getenv("OZ_FLOW_CONTROL_FAKE_OZ") != "1" {
		return
	}
	args := os.Args
	for i, arg := range args {
		if arg == "--" {
			flowControlFakeOzMain(args[i+1:])
			os.Exit(0)
		}
	}
	os.Exit(1)
}

// flowControlFakeOzMain implements list and validate for temporary test repositories.
func flowControlFakeOzMain(args []string) {
	switch {
	case len(args) == 2 && args[0] == "list" && args[1] == "--json":
		names, err := flowControlActiveChangeNames(".")
		if err != nil {
			os.Exit(1)
		}
		var parts []string
		for _, name := range names {
			parts = append(parts, fmt.Sprintf(`{"name":%q}`, name))
		}
		_, _ = os.Stdout.WriteString(`{"changes":[` + strings.Join(parts, ",") + "]}\n")
	case len(args) == 3 && args[0] == "validate" && args[2] == "--json":
		_, _ = os.Stdout.WriteString(`{"valid":true,"errors":[]}` + "\n")
	default:
		os.Exit(1)
	}
	os.Exit(0)
}

// flowControlActiveChangeNames scans active change directories like the real oz list command.
func flowControlActiveChangeNames(repo string) ([]string, error) {
	root := filepath.Join(repo, "docs", "changes")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "archive" || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}
