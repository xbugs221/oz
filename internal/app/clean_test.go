// Package app tests wo clean behavior for removing failed and abnormal runtime state.
package app

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCleanRemovesFailedInterruptedBlockedAbortedRuns verifies clean deletes terminal runs.
func TestCleanRemovesFailedInterruptedBlockedAbortedRuns(t *testing.T) {
	repo := gitRepo(t)
	for _, tc := range []struct {
		runID  string
		status string
		stage  string
	}{
		{"run-failed", statusFailed, "execution"},
		{"run-interrupted", statusInterrupted, "execution"},
		{"run-blocked", statusBlocked, statusBlocked},
		{"run-validation", statusValidationBlocked, statusValidationBlocked},
		{"run-aborted", statusAborted, "execution"},
		{"run-aborted2", "aborted", "execution"},
	} {
		state := State{RunID: tc.runID, ChangeName: "demo", Sealed: true, Status: tc.status, Stage: tc.stage, Workflow: DefaultWorkflowConfig()}
		if err := saveState(repo, state); err != nil {
			t.Fatal(err)
		}
	}

	result, err := CleanRuntimeState(repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.CleanedRuns != 6 {
		t.Fatalf("cleaned runs = %d, want 6", result.CleanedRuns)
	}
	if result.CleanedBatches != 0 {
		t.Fatalf("cleaned batches = %d, want 0", result.CleanedBatches)
	}

	// Verify directories are gone.
	for _, tc := range []string{"run-failed", "run-interrupted", "run-blocked", "run-validation", "run-aborted", "run-aborted2"} {
		if _, err := os.Stat(runDir(repo, tc)); !os.IsNotExist(err) {
			t.Fatalf("run %s still exists", tc)
		}
	}
}

// TestCleanPreservesDoneAndArchivedRuns verifies clean keeps useful history.
func TestCleanPreservesDoneAndArchivedRuns(t *testing.T) {
	repo := gitRepo(t)

	doneState := State{RunID: "run-done", ChangeName: "demo", Sealed: true, Status: statusDone, Stage: "done", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, doneState); err != nil {
		t.Fatal(err)
	}
	archivedState := State{RunID: "run-archived", ChangeName: "demo", Sealed: true, Status: statusArchived, Stage: "archive", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, archivedState); err != nil {
		t.Fatal(err)
	}

	result, err := CleanRuntimeState(repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.CleanedRuns != 0 {
		t.Fatalf("cleaned runs = %d, want 0", result.CleanedRuns)
	}
	for _, id := range []string{"run-done", "run-archived"} {
		if _, err := os.Stat(runDir(repo, id)); err != nil {
			t.Fatalf("run %s was removed: %v", id, err)
		}
	}
}

// TestCleanSkipsActiveLockedRunningRun verifies active lock protects running runs.
func TestCleanSkipsActiveLockedRunningRun(t *testing.T) {
	repo := gitRepo(t)
	runID := "run-running-locked"
	state := State{RunID: runID, ChangeName: "demo", Sealed: true, Status: statusRunning, Stage: "execution", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	mustWriteLock(t, repo, runID, LockInfo{PID: os.Getpid(), RunID: runID, StartedAt: time.Now().UTC().Format(time.RFC3339Nano)})

	result, err := CleanRuntimeState(repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.CleanedRuns != 0 {
		t.Fatalf("cleaned runs = %d, want 0", result.CleanedRuns)
	}
	if result.SkippedRunning != 1 {
		t.Fatalf("skipped running = %d, want 1", result.SkippedRunning)
	}
	if _, err := os.Stat(runDir(repo, runID)); err != nil {
		t.Fatalf("run with active lock was removed: %v", err)
	}

	// Verify output includes the skipped-running message even when nothing was cleaned.
	var stdout bytes.Buffer
	if err := runClean(&stdout, repo); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	if !containsChinese(got, "已跳过 1 个仍在运行的任务") {
		t.Fatalf("output missing skipped-running message:\n%s", got)
	}
	if containsChinese(got, "没有需要清理的失败或异常运行态") {
		t.Fatalf("output must not show empty-state when skipped runs exist:\n%s", got)
	}
}

// TestCleanCleansFailedBatchAndReferencedRuns verifies batch cleanup with linked runs.
func TestCleanCleansFailedBatchAndReferencedRuns(t *testing.T) {
	repo := gitRepo(t)

	// Create two runs referenced by a failed batch.
	s1 := State{RunID: "run-a", ChangeName: "1-a", Sealed: true, Status: statusFailed, Stage: "execution", Workflow: DefaultWorkflowConfig()}
	s2 := State{RunID: "run-b", ChangeName: "2-b", Sealed: true, Status: statusFailed, Stage: "execution", Workflow: DefaultWorkflowConfig()}
	for _, s := range []State{s1, s2} {
		if err := saveState(repo, s); err != nil {
			t.Fatal(err)
		}
	}

	batch := BatchState{
		BatchID:      "batch-failed",
		Status:       batchStatusFailed,
		Changes:      []string{"1-a", "2-b"},
		CurrentIndex: 1,
		RunIDs:       map[string]string{"1-a": "run-a", "2-b": "run-b"},
		FailedChange: "2-b",
		FailedRunID:  "run-b",
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	result, err := CleanRuntimeState(repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.CleanedBatches != 1 {
		t.Fatalf("cleaned batches = %d, want 1", result.CleanedBatches)
	}
	// run-a is cleaned in phase 1 (it's a standalone failed run), run-b is cleaned as batch ref.
	if result.CleanedRuns != 2 {
		t.Fatalf("cleaned runs = %d, want 2", result.CleanedRuns)
	}

	for _, id := range []string{"run-a", "run-b"} {
		if _, err := os.Stat(runDir(repo, id)); !os.IsNotExist(err) {
			t.Fatalf("run %s still exists", id)
		}
	}
	if _, err := os.Stat(batchDir(repo, "batch-failed")); !os.IsNotExist(err) {
		t.Fatalf("batch directory still exists")
	}
}

// TestCleanSkipsBatchWithActiveReferencedRun verifies batch with active run is preserved.
func TestCleanSkipsBatchWithActiveReferencedRun(t *testing.T) {
	repo := gitRepo(t)

	s1 := State{RunID: "run-active", ChangeName: "1-a", Sealed: true, Status: statusFailed, Stage: "execution", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, s1); err != nil {
		t.Fatal(err)
	}
	mustWriteLock(t, repo, "run-active", LockInfo{PID: os.Getpid(), RunID: "run-active", StartedAt: time.Now().UTC().Format(time.RFC3339Nano)})

	batch := BatchState{
		BatchID:      "batch-failed",
		Status:       batchStatusFailed,
		Changes:      []string{"1-a"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{"1-a": "run-active"},
		FailedChange: "1-a",
		FailedRunID:  "run-active",
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	result, err := CleanRuntimeState(repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.CleanedBatches != 0 {
		t.Fatalf("cleaned batches = %d, want 0", result.CleanedBatches)
	}
	if result.CleanedRuns != 0 {
		t.Fatalf("cleaned runs = %d, want 0", result.CleanedRuns)
	}
	if result.SkippedRunning != 2 {
		t.Fatalf("skipped running = %d, want 2 (locked run + batch with locked run)", result.SkippedRunning)
	}

	if _, err := os.Stat(batchDir(repo, "batch-failed")); err != nil {
		t.Fatalf("batch dir was removed: %v", err)
	}
	if _, err := os.Stat(runDir(repo, "run-active")); err != nil {
		t.Fatalf("active run was removed: %v", err)
	}
}

// TestCleanSkipsBatchWithMixedRuns verifies that when a cleanable batch references
// both an active-locked run and a non-locked cleanable run, the ENTIRE batch and
// ALL its referenced runs are preserved (design.md: atomic batch preservation).
func TestCleanSkipsBatchWithMixedRuns(t *testing.T) {
	repo := gitRepo(t)

	// run-active: cleanable status, active lock.
	runActive := "run-active"
	sActive := State{RunID: runActive, ChangeName: "1-a", Sealed: true, Status: statusFailed, Stage: "execution", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, sActive); err != nil {
		t.Fatal(err)
	}
	mustWriteLock(t, repo, runActive, LockInfo{PID: os.Getpid(), RunID: runActive, StartedAt: time.Now().UTC().Format(time.RFC3339Nano)})

	// run-failed: cleanable status, no lock — should be protected by the batch.
	runFailed := "run-failed"
	sFailed := State{RunID: runFailed, ChangeName: "2-b", Sealed: true, Status: statusFailed, Stage: "execution", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, sFailed); err != nil {
		t.Fatal(err)
	}

	// Batch references both.
	batch := BatchState{
		BatchID:      "batch-failed",
		Status:       batchStatusFailed,
		Changes:      []string{"1-a", "2-b"},
		CurrentIndex: 1,
		RunIDs:       map[string]string{"1-a": runActive, "2-b": runFailed},
		FailedChange: "2-b",
		FailedRunID:  runFailed,
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	result, err := CleanRuntimeState(repo)
	if err != nil {
		t.Fatal(err)
	}
	// Neither the batch nor any referenced run should be cleaned.
	if result.CleanedBatches != 0 {
		t.Fatalf("cleaned batches = %d, want 0 (batch must be preserved)", result.CleanedBatches)
	}
	if result.CleanedRuns != 0 {
		t.Fatalf("cleaned runs = %d, want 0 (all runs must be preserved)", result.CleanedRuns)
	}
	// SkippedRunning: 1 for run-active (own lock), 1 for batch (active referenced run).
	if result.SkippedRunning != 2 {
		t.Fatalf("skipped running = %d, want 2 (run-active + batch)", result.SkippedRunning)
	}

	// All directories must still exist.
	for _, id := range []string{runActive, runFailed} {
		if _, err := os.Stat(runDir(repo, id)); err != nil {
			t.Fatalf("run %s was removed: %v", id, err)
		}
	}
	if _, err := os.Stat(batchDir(repo, "batch-failed")); err != nil {
		t.Fatalf("batch directory was removed: %v", err)
	}

	// Output should mention skipped tasks.
	var stdout bytes.Buffer
	if err := runClean(&stdout, repo); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	if !containsChinese(got, "已跳过 2 个仍在运行的任务") {
		t.Fatalf("output missing skip count:\n%s", got)
	}
}

// TestCleanFailedBatchPreservesReferencedRunningRun verifies failed batch cleanup
// does not delete an unlocked running run or its agent session records.
func TestCleanFailedBatchPreservesReferencedRunningRun(t *testing.T) {
	repo := gitRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))

	runID := "run-running-unlocked"
	sessionID := "019erunref-0000-7000-8000-000000000000"
	state := State{
		RunID:      runID,
		ChangeName: "1-a",
		Sealed:     true,
		Status:     statusRunning,
		Stage:      "execution",
		Sessions:   map[string]string{"codex:executor": sessionID},
		Workflow:   DefaultWorkflowConfig(),
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	batch := BatchState{
		BatchID:      "batch-failed",
		Status:       batchStatusFailed,
		Changes:      []string{"1-a"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{"1-a": runID},
		FailedChange: "1-a",
		FailedRunID:  runID,
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}
	codexFile := filepath.Join(home, ".codex", "sessions", "2026", "05", "26", "rollout-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(codexFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codexFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := CleanRuntimeState(repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.CleanedBatches != 1 {
		t.Fatalf("cleaned batches = %d, want 1", result.CleanedBatches)
	}
	if result.CleanedRuns != 0 {
		t.Fatalf("cleaned runs = %d, want 0 (running run must be preserved)", result.CleanedRuns)
	}
	if result.CleanedAgentRecords != 0 {
		t.Fatalf("cleaned agent records = %d, want 0 (running run session must be protected)", result.CleanedAgentRecords)
	}
	if _, err := os.Stat(runDir(repo, runID)); err != nil {
		t.Fatalf("running run was removed: %v", err)
	}
	if _, err := os.Stat(batchDir(repo, "batch-failed")); !os.IsNotExist(err) {
		t.Fatalf("failed batch still exists")
	}
	if _, err := os.Stat(codexFile); err != nil {
		t.Fatalf("running run codex file was removed: %v", err)
	}
}

// TestCleanRemovesCorruptedRunState verifies broken state.json is cleaned.
func TestCleanRemovesCorruptedRunState(t *testing.T) {
	repo := gitRepo(t)

	// Missing state.json.
	missingDir := runDir(repo, "run-missing")
	if err := os.MkdirAll(missingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(missingDir, "artifact.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Corrupt state.json.
	corruptDir := runDir(repo, "run-corrupt")
	if err := os.MkdirAll(corruptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corruptDir, "state.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := CleanRuntimeState(repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.CleanedRuns != 2 {
		t.Fatalf("cleaned runs = %d, want 2", result.CleanedRuns)
	}
	for _, id := range []string{"run-missing", "run-corrupt"} {
		if _, err := os.Stat(runDir(repo, id)); !os.IsNotExist(err) {
			t.Fatalf("run %s still exists", id)
		}
	}
}

// TestCleanRemovesCorruptedBatchState verifies broken batch state.json is cleaned.
func TestCleanRemovesCorruptedBatchState(t *testing.T) {
	repo := gitRepo(t)

	// Missing batch state.json.
	missingDir := batchDir(repo, "batch-missing")
	if err := os.MkdirAll(missingDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Corrupt batch state.json.
	corruptDir := batchDir(repo, "batch-corrupt")
	if err := os.MkdirAll(corruptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corruptDir, "state.json"), []byte("bad"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := CleanRuntimeState(repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.CleanedBatches != 2 {
		t.Fatalf("cleaned batches = %d, want 2", result.CleanedBatches)
	}
	for _, id := range []string{"batch-missing", "batch-corrupt"} {
		if _, err := os.Stat(batchDir(repo, id)); !os.IsNotExist(err) {
			t.Fatalf("batch %s still exists", id)
		}
	}
}

// TestCleanOutputNoCleanableObjects verifies empty-state output.
func TestCleanOutputNoCleanableObjects(t *testing.T) {
	repo := gitRepo(t)
	var stdout bytes.Buffer
	if err := runClean(&stdout, repo); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	if !containsChinese(got, "没有需要清理的失败或异常运行态") {
		t.Fatalf("output missing empty-state message:\n%s", got)
	}
	if !containsChinese(got, "该操作仅检查当前项目 wo 历史记录，不回滚代码改动") {
		t.Fatalf("output missing code-change disclaimer:\n%s", got)
	}
}

// TestCleanOutputWithCleanedObjects verifies clean summary output.
func TestCleanOutputWithCleanedObjects(t *testing.T) {
	repo := gitRepo(t)

	// Create a failed run.
	state := State{RunID: "run-failed", ChangeName: "demo", Sealed: true, Status: statusFailed, Stage: "execution", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := runClean(&stdout, repo); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	if !containsChinese(got, "已清理 0 个批量任务、1 个工作流") {
		t.Fatalf("output missing clean count:\n%s", got)
	}
	if !containsChinese(got, "范围: 当前项目") {
		t.Fatalf("output missing scope:\n%s", got)
	}
	if !containsChinese(got, "该操作仅删除 wo 历史记录，不回滚代码改动") {
		t.Fatalf("output missing code-change disclaimer:\n%s", got)
	}
}

// TestCleanCLIHelpIncludesClean verifies help shows wo clean.
func TestCleanCLIHelpIncludesClean(t *testing.T) {
	repo := gitRepo(t)
	t.Chdir(repo)
	var stdout bytes.Buffer
	Run([]string{"--help"}, nil, &stdout, &bytes.Buffer{})
	got := stdout.String()
	if !containsChinese(got, "wo clean") {
		t.Fatalf("help output missing wo clean:\n%s", got)
	}
}

// TestCleanCLIUsageError verifies extra args are rejected.
func TestCleanCLIUsageError(t *testing.T) {
	repo := gitRepo(t)
	t.Chdir(repo)
	var stdout bytes.Buffer
	err := Run([]string{"clean", "extra"}, nil, &stdout, &bytes.Buffer{})
	if err == nil {
		t.Fatal("wo clean with extra args succeeded, want usage error")
	}
}

// TestCleanAbortedBatchCleaned verifies aborted batch is cleaned.
func TestCleanAbortedBatchCleaned(t *testing.T) {
	repo := gitRepo(t)

	batch := BatchState{
		BatchID: "batch-aborted",
		Status:  batchStatusAborted,
		Changes: []string{"1-a"},
		RunIDs:  map[string]string{},
		Error:   "用户已中止",
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	result, err := CleanRuntimeState(repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.CleanedBatches != 1 {
		t.Fatalf("cleaned batches = %d, want 1", result.CleanedBatches)
	}
	if _, err := os.Stat(batchDir(repo, "batch-aborted")); !os.IsNotExist(err) {
		t.Fatalf("aborted batch still exists")
	}
}

// TestCleanAbortedBatchCleansReferencedRuns verifies aborted batch with runs.
func TestCleanAbortedBatchCleansReferencedRuns(t *testing.T) {
	repo := gitRepo(t)

	runID := "run-aborted-ref"
	state := State{RunID: runID, ChangeName: "1-a", Sealed: true, Status: "aborted", Stage: "execution", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}

	batch := BatchState{
		BatchID: "batch-aborted",
		Status:  batchStatusAborted,
		Changes: []string{"1-a"},
		RunIDs:  map[string]string{"1-a": runID},
		Error:   "用户已中止",
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	result, err := CleanRuntimeState(repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.CleanedBatches != 1 {
		t.Fatalf("cleaned batches = %d, want 1", result.CleanedBatches)
	}
	if result.CleanedRuns != 1 {
		t.Fatalf("cleaned runs = %d, want 1", result.CleanedRuns)
	}
	if _, err := os.Stat(runDir(repo, runID)); !os.IsNotExist(err) {
		t.Fatalf("referenced run still exists")
	}
}

// TestCleanPreservesRunningBatch verifies running batches are not cleaned.
func TestCleanPreservesRunningBatch(t *testing.T) {
	repo := gitRepo(t)

	batch := BatchState{
		BatchID: "batch-running",
		Status:  batchStatusRunning,
		Changes: []string{"1-a"},
		RunIDs:  map[string]string{},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	result, err := CleanRuntimeState(repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.CleanedBatches != 0 {
		t.Fatalf("cleaned batches = %d, want 0", result.CleanedBatches)
	}
	if _, err := os.Stat(batchDir(repo, "batch-running")); err != nil {
		t.Fatalf("running batch was removed: %v", err)
	}
}

// TestCleanPreservesDoneBatch verifies done batches are not cleaned.
func TestCleanPreservesDoneBatch(t *testing.T) {
	repo := gitRepo(t)

	batch := BatchState{
		BatchID:      "batch-done",
		Status:       batchStatusDone,
		Changes:      []string{"1-a", "2-b"},
		CurrentIndex: 2,
		RunIDs:       map[string]string{"1-a": "run-a", "2-b": "run-b"},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	result, err := CleanRuntimeState(repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.CleanedBatches != 0 {
		t.Fatalf("done batch was cleaned")
	}
	if _, err := os.Stat(batchDir(repo, "batch-done")); err != nil {
		t.Fatalf("done batch was removed: %v", err)
	}
}

// TestCleanBatchReferencedRunAlreadyCleaned avoids double counting.
func TestCleanBatchReferencedRunAlreadyCleaned(t *testing.T) {
	repo := gitRepo(t)

	// A standalone failed run that will be cleaned in phase 1.
	s1 := State{RunID: "run-standalone", ChangeName: "1-a", Sealed: true, Status: statusFailed, Stage: "execution", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, s1); err != nil {
		t.Fatal(err)
	}

	// A batch that also references this run via failed_run_id.
	batch := BatchState{
		BatchID:      "batch-failed",
		Status:       batchStatusFailed,
		Changes:      []string{"1-a"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{"1-a": "run-standalone"},
		FailedChange: "1-a",
		FailedRunID:  "run-standalone",
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	result, err := CleanRuntimeState(repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.CleanedBatches != 1 {
		t.Fatalf("cleaned batches = %d, want 1", result.CleanedBatches)
	}
	if result.CleanedRuns != 1 {
		t.Fatalf("cleaned runs = %d, want 1 (run counted once despite batch ref)", result.CleanedRuns)
	}
}

// TestCleanBatchReferencingMissingRuns verifies that a failed batch referencing
// non-existent runs only counts the batch, not the phantom runs (design.md: 缺失目录只计为已不存在).
func TestCleanBatchReferencingMissingRuns(t *testing.T) {
	repo := gitRepo(t)

	// Create a failed batch whose failed_run_id and run_ids all point to non-existent runs.
	batch := BatchState{
		BatchID:      "batch-failed-missing-runs",
		Status:       batchStatusFailed,
		Changes:      []string{"1-a", "2-b"},
		CurrentIndex: 1,
		RunIDs:       map[string]string{"1-a": "run-never-created-a", "2-b": "run-never-created-b"},
		FailedChange: "2-b",
		FailedRunID:  "run-never-created-b",
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	result, err := CleanRuntimeState(repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.CleanedBatches != 1 {
		t.Fatalf("cleaned batches = %d, want 1", result.CleanedBatches)
	}
	if result.CleanedRuns != 0 {
		t.Fatalf("cleaned runs = %d, want 0 (phantom runs must not be counted)", result.CleanedRuns)
	}
	if _, err := os.Stat(batchDir(repo, "batch-failed-missing-runs")); !os.IsNotExist(err) {
		t.Fatalf("batch directory still exists")
	}
}

// TestCleanOutputWithSkippedRunning verifies skip count in output.
func TestCleanOutputWithSkippedRunning(t *testing.T) {
	repo := gitRepo(t)

	// A failed run with active lock.
	runID := "run-locked"
	state := State{RunID: runID, ChangeName: "demo", Sealed: true, Status: statusFailed, Stage: "execution", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	mustWriteLock(t, repo, runID, LockInfo{PID: os.Getpid(), RunID: runID, StartedAt: time.Now().UTC().Format(time.RFC3339Nano)})

	// Also a cleanable failed run.
	s2 := State{RunID: "run-failed", ChangeName: "1-b", Sealed: true, Status: statusFailed, Stage: "execution", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, s2); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := runClean(&stdout, repo); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	if !containsChinese(got, "已清理 0 个批量任务、1 个工作流") {
		t.Fatalf("output missing clean count:\n%s", got)
	}
	if !containsChinese(got, "已跳过 1 个仍在运行的任务") {
		t.Fatalf("output missing skip count:\n%s", got)
	}
}

// TestCleanRemovesAgentSessionRecords verifies failed runs clean referenced Codex/Pi records.
func TestCleanRemovesAgentSessionRecords(t *testing.T) {
	repo := gitRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))

	codexID := "019eaaaa-bbbb-7ccc-8ddd-eeeeeeeeeeee"
	piID := "019effff-1111-7222-8333-444444444444"
	state := State{
		RunID:      "run-failed-agent",
		ChangeName: "demo",
		Sealed:     true,
		Status:     statusFailed,
		Stage:      "execution",
		Sessions:   map[string]string{"codex:executor": codexID, "pi:archiver": piID},
		Workflow:   DefaultWorkflowConfig(),
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}

	codexFile := filepath.Join(home, ".codex", "sessions", "2026", "05", "26", "rollout-"+codexID+".jsonl")
	nearCodexFile := filepath.Join(home, ".codex", "sessions", "2026", "05", "26", "rollout-"+codexID[:len(codexID)-1]+".jsonl")
	piFile := filepath.Join(home, ".pi", "agent", "sessions", "repo", "turn_"+piID+".jsonl")
	for _, path := range []string{codexFile, nearCodexFile, piFile} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	result, err := CleanRuntimeState(repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.CleanedAgentRecords != 2 {
		t.Fatalf("cleaned agent records = %d, want 2", result.CleanedAgentRecords)
	}
	for _, path := range []string{codexFile, piFile} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("agent record still exists: %s", path)
		}
	}
	if _, err := os.Stat(nearCodexFile); err != nil {
		t.Fatalf("near-match codex file was removed: %v", err)
	}
}

// TestCleanPreservesSharedAgentSessionRecords verifies sessions used by kept runs are protected.
func TestCleanPreservesSharedAgentSessionRecords(t *testing.T) {
	repo := gitRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))

	sharedID := "019eshared-0000-7000-8000-000000000000"
	failed := State{RunID: "run-failed", ChangeName: "demo", Sealed: true, Status: statusFailed, Stage: "execution", Sessions: map[string]string{"codex:executor": sharedID}, Workflow: DefaultWorkflowConfig()}
	done := State{RunID: "run-done", ChangeName: "demo", Sealed: true, Status: statusDone, Stage: "done", Sessions: map[string]string{"codex:executor": sharedID}, Workflow: DefaultWorkflowConfig()}
	for _, state := range []State{failed, done} {
		if err := saveState(repo, state); err != nil {
			t.Fatal(err)
		}
	}
	codexFile := filepath.Join(home, ".codex", "sessions", "2026", "05", "26", "rollout-"+sharedID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(codexFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codexFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := CleanRuntimeState(repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.CleanedRuns != 1 || result.CleanedAgentRecords != 0 {
		t.Fatalf("result = %+v, want one run and no shared agent cleanup", result)
	}
	if _, err := os.Stat(codexFile); err != nil {
		t.Fatalf("shared codex file was removed: %v", err)
	}
}

// TestCleanBatchReferencedRunRemovesAgentSession verifies batch cleanup collects run sessions.
func TestCleanBatchReferencedRunRemovesAgentSession(t *testing.T) {
	repo := gitRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))

	sessionID := "019ebatch-0000-7000-8000-000000000000"
	state := State{RunID: "run-batch", ChangeName: "1-a", Sealed: true, Status: statusFailed, Stage: "execution", Sessions: map[string]string{"codex:executor": sessionID}, Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	batch := BatchState{BatchID: "batch-failed", Status: batchStatusFailed, Changes: []string{"1-a"}, RunIDs: map[string]string{"1-a": "run-batch"}, FailedRunID: "run-batch"}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}
	codexFile := filepath.Join(home, ".codex", "sessions", "2026", "05", "26", "rollout-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(codexFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codexFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := CleanRuntimeState(repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.CleanedBatches != 1 || result.CleanedRuns != 1 || result.CleanedAgentRecords != 1 {
		t.Fatalf("result = %+v, want batch/run/agent cleanup", result)
	}
	if _, err := os.Stat(codexFile); !os.IsNotExist(err) {
		t.Fatalf("batch run codex file still exists")
	}
}

// TestCleanPiSQLiteBestEffort verifies known SQLite schemas are cleaned and unknown schemas skipped.
func TestCleanPiSQLiteBestEffort(t *testing.T) {
	repo := gitRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	sessionID := "019esqlite-0000-7000-8000-000000000000"
	state := State{RunID: "run-failed-sqlite", ChangeName: "demo", Sealed: true, Status: statusFailed, Stage: "execution", Sessions: map[string]string{"pi:archiver": sessionID}, Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(home, ".pi", "agent", "session-index.sqlite")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	runPythonSQLite(t, dbPath, `
import sqlite3
import sys

db = sqlite3.connect(sys.argv[1])
db.execute("create table sessions(id text primary key, cwd text)")
db.execute("create table messages(id integer primary key, session_id text, body text)")
db.execute("create table unrelated(id text primary key)")
db.execute("insert into sessions(id, cwd) values ('019esqlite-0000-7000-8000-000000000000', 'repo')")
db.execute("insert into messages(session_id, body) values ('019esqlite-0000-7000-8000-000000000000', 'clean')")
db.execute("insert into unrelated(id) values ('019esqlite-0000-7000-8000-000000000000')")
db.commit()
db.close()
`)

	result, err := CleanRuntimeState(repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.CleanedAgentRecords != 2 {
		t.Fatalf("cleaned agent records = %d, want 2 table groups", result.CleanedAgentRecords)
	}
	out := runPythonSQLite(t, dbPath, `
import sqlite3
import sys

db = sqlite3.connect(sys.argv[1])
sessions = db.execute("select count(*) from sessions where id = '019esqlite-0000-7000-8000-000000000000'").fetchone()[0]
messages = db.execute("select count(*) from messages where session_id = '019esqlite-0000-7000-8000-000000000000'").fetchone()[0]
unrelated = db.execute("select count(*) from unrelated").fetchone()[0]
db.close()
print(f"{sessions} {messages} {unrelated}")
`)
	if strings.TrimSpace(out) != "0 0 1" {
		t.Fatalf("sqlite rows = %q, want 0 0 1", strings.TrimSpace(out))
	}
}

// runPythonSQLite executes a small sqlite3 script for test setup and assertions.
func runPythonSQLite(t *testing.T, dbPath, script string) string {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 required for sqlite test")
	}
	cmd := exec.Command(python, "-", dbPath)
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python sqlite script failed: %v\n%s", err, out)
	}
	return string(out)
}

// containsChinese is a simple helper for checking Chinese string containment in test output.
func containsChinese(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
