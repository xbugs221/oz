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

// TestLoopArchivesFailedBatchAndStartsContinuation verifies loop continues from unfinished active changes.
func TestLoopArchivesFailedBatchAndStartsContinuation(t *testing.T) {
	repo := gitRepo(t)
	installFlowControlFakeOz(t)
	for _, name := range []string{"1-a", "2-b", "3-c"} {
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
