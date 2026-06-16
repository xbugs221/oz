// Package app tests oz flow clean plan boundaries before destructive cleanup.
package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCleanRuntimeDryRunDoesNotDeleteRun verifies dry-run returns the plan summary without removing runtime state.
func TestCleanRuntimeDryRunDoesNotDeleteRun(t *testing.T) {
	repo := cleanPlanRepo(t)
	cleanPlanSaveRun(t, repo, "failed-dry-run", statusFailed)

	result, err := CleanRuntimeStateWithOptions(repo, CleanOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.CleanedRuns != 1 {
		t.Fatalf("CleanedRuns = %d, want 1", result.CleanedRuns)
	}
	if _, err := os.Stat(runDir(repo, "failed-dry-run")); err != nil {
		t.Fatalf("dry-run removed run directory: %v", err)
	}
}

// TestCleanPlanApplyMatchesDryRunSummary verifies apply uses the same decisions produced by BuildCleanPlan.
func TestCleanPlanApplyMatchesDryRunSummary(t *testing.T) {
	repo := cleanPlanRepo(t)
	cleanPlanSaveRun(t, repo, "failed-apply", statusFailed)
	if err := saveBatchState(repo, BatchState{BatchID: "batch-failed", Status: batchStatusFailed, FailedRunID: "failed-apply", RunIDs: map[string]string{}}); err != nil {
		t.Fatal(err)
	}

	plan, err := BuildCleanPlan(repo, CleanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := plan.Summary(CleanOptions{})
	got, err := ApplyCleanPlan(plan, CleanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.CleanedRuns != want.CleanedRuns || got.CleanedBatches != want.CleanedBatches {
		t.Fatalf("apply result = %#v, want summary %#v", got, want)
	}
	if _, err := os.Stat(runDir(repo, "failed-apply")); !os.IsNotExist(err) {
		t.Fatalf("apply did not remove run directory: %v", err)
	}
	if _, err := os.Stat(batchDir(repo, "batch-failed")); !os.IsNotExist(err) {
		t.Fatalf("apply did not remove batch directory: %v", err)
	}
}

// TestBuildCleanPlanProtectsActiveLockedRun verifies active locks keep cleanable runs out of delete actions.
func TestBuildCleanPlanProtectsActiveLockedRun(t *testing.T) {
	repo := cleanPlanRepo(t)
	cleanPlanSaveRun(t, repo, "locked-failed", statusFailed)
	cleanPlanWriteActiveLock(t, repo, "locked-failed")

	plan, err := BuildCleanPlan(repo, CleanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	item := cleanPlanRunItem(t, plan, "locked-failed")
	if item.Action != cleanActionProtect || item.Reason != "active_lock" {
		t.Fatalf("locked run item = %#v, want active_lock protect", item)
	}
}

// TestBuildCleanPlanDeletesCorruptRun verifies corrupt state is planned for cleanup.
func TestBuildCleanPlanDeletesCorruptRun(t *testing.T) {
	repo := cleanPlanRepo(t)
	dir := runDir(repo, "corrupt-run")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte("{not-json"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := BuildCleanPlan(repo, CleanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	item := cleanPlanRunItem(t, plan, "corrupt-run")
	if item.Action != cleanActionDelete || item.Reason != "corrupt_or_missing_state" {
		t.Fatalf("corrupt run item = %#v, want corrupt delete", item)
	}
}

// cleanPlanRepo creates an isolated repository runtime root for clean tests.
func cleanPlanRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	return t.TempDir()
}

// cleanPlanSaveRun writes a minimal run state.
func cleanPlanSaveRun(t *testing.T, repo, runID, status string) {
	t.Helper()
	state := State{RunID: runID, ChangeName: "clean-plan", Status: status, Sessions: map[string]string{}, Stages: map[string]string{}, Paths: map[string]string{}}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
}

// cleanPlanWriteActiveLock writes a lock for the current live test process.
func cleanPlanWriteActiveLock(t *testing.T, repo, runID string) {
	t.Helper()
	hostname, _ := os.Hostname()
	lock := LockInfo{PID: os.Getpid(), Hostname: hostname, RunID: runID, StartedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	data, err := json.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir(repo, runID), "lock"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// cleanPlanRunItem finds a run item by ID.
func cleanPlanRunItem(t *testing.T, plan CleanPlan, runID string) CleanRunPlanItem {
	t.Helper()
	for _, item := range plan.Runs {
		if item.RunID == runID {
			return item
		}
	}
	t.Fatalf("run %s not found in plan %#v", runID, plan.Runs)
	return CleanRunPlanItem{}
}
