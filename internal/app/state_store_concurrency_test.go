// Package app tests monotonic and atomic workflow state persistence.
package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestSaveStateDoesNotRegressWorkerHeartbeatOrExit preserves lifecycle metadata across stale saves.
func TestSaveStateDoesNotRegressWorkerHeartbeatOrExit(t *testing.T) {
	repo := gitRepo(t)
	const runID = "monotonic-worker-state"
	started := time.Now().Add(-time.Hour)
	stale := workerRuntimeTestState(runID)
	stale.Worker = newWorkerRuntime("worker.log", started)
	if err := saveState(repo, stale); err != nil {
		t.Fatal(err)
	}
	freshHeartbeat := started.Add(30 * time.Minute)
	if err := recordRunWorkerHeartbeat(repo, runID, freshHeartbeat); err != nil {
		t.Fatal(err)
	}
	if err := recordRunWorkerExit(repo, runID, workerExitError, "worker stopped", freshHeartbeat.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	stale.Stage = "review_1"
	if err := saveState(repo, stale); err != nil {
		t.Fatal(err)
	}
	persisted, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Worker == nil {
		t.Fatal("worker metadata was lost")
	}
	if persisted.Worker.LastHeartbeatAt != timestampUTC(freshHeartbeat.Add(time.Minute)) {
		t.Fatalf("heartbeat regressed to %q", persisted.Worker.LastHeartbeatAt)
	}
	if persisted.Worker.Exit != workerExitError || persisted.Worker.Error != "worker stopped" {
		t.Fatalf("terminal worker state regressed: %#v", persisted.Worker)
	}
}

// TestConcurrentRawStateReadsAlwaysSeeValidJSON verifies atomic replacement hides partial writes.
func TestConcurrentRawStateReadsAlwaysSeeValidJSON(t *testing.T) {
	repo := gitRepo(t)
	const runID = "atomic-json-state"
	state := workerRuntimeTestState(runID)
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(runDir(repo, runID), "state.json")
	done := make(chan struct{})
	errs := make(chan error, 16)
	var readers sync.WaitGroup
	for range 8 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				data, err := os.ReadFile(path)
				if err != nil {
					errs <- err
					return
				}
				var decoded State
				if err := json.Unmarshal(data, &decoded); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	for i := range 300 {
		state.Stage = "stage-" + time.Unix(int64(i), 0).UTC().Format("150405")
		if err := saveState(repo, state); err != nil {
			t.Fatal(err)
		}
	}
	close(done)
	readers.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent reader saw invalid state: %v", err)
	}
}
