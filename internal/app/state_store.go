// Package app stores sealed workflow run state in per-run JSON files.
package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var stateFileMu sync.Mutex

// saveState writes durable workflow state as pretty JSON.
func saveState(repo string, state State) error {
	stateFileMu.Lock()
	defer stateFileMu.Unlock()
	if err := validateRunID(state.RunID); err != nil {
		return err
	}
	unlock, err := lockRunStateFile(repo, state.RunID)
	if err != nil {
		return err
	}
	defer func() { _ = unlock() }()
	state.Worker = latestWorkerForSave(filepath.Join(runDir(repo, state.RunID), "state.json"), state.RunID, state.Worker)
	normalizeStateMaps(&state)
	refreshStateProcesses(&state)
	return writeStateFileLocked(repo, state.RunID, state)
}

// loadState reads durable workflow state for a run id.
func loadState(repo, runID string) (State, error) {
	stateFileMu.Lock()
	defer stateFileMu.Unlock()
	if err := validateRunID(runID); err != nil {
		return State{}, err
	}
	unlock, err := lockRunStateFile(repo, runID)
	if err != nil {
		return State{}, err
	}
	defer func() { _ = unlock() }()
	data, err := os.ReadFile(filepath.Join(runDir(repo, runID), "state.json"))
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, err
	}
	if err := validateRunID(state.RunID); err != nil {
		return State{}, err
	}
	if state.RunID != runID {
		return State{}, fmt.Errorf("state run_id %q does not match requested run %q", state.RunID, runID)
	}
	normalizeStateMaps(&state)
	return state, nil
}

// mergeState applies a small mutation to the latest durable state under one lock.
func mergeState(repo, runID string, mutate func(*State)) error {
	stateFileMu.Lock()
	defer stateFileMu.Unlock()
	if err := validateRunID(runID); err != nil {
		return err
	}
	unlock, err := lockRunStateFile(repo, runID)
	if err != nil {
		return err
	}
	defer func() { _ = unlock() }()
	path := filepath.Join(runDir(repo, runID), "state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	if state.RunID != runID {
		return fmt.Errorf("state run_id %q does not match requested run %q", state.RunID, runID)
	}
	normalizeStateMaps(&state)
	if mutate != nil {
		mutate(&state)
	}
	if state.RunID != runID {
		return fmt.Errorf("state mutation changed run_id from %q to %q", runID, state.RunID)
	}
	normalizeStateMaps(&state)
	refreshStateProcesses(&state)
	return writeStateFileLocked(repo, runID, state)
}

// writeStateFileLocked writes state.json while the caller holds both process and file locks.
func writeStateFileLocked(repo, runID string, state State) error {
	if state.RunID != runID {
		return fmt.Errorf("state run_id %q does not match write run %q", state.RunID, runID)
	}
	if err := os.MkdirAll(runDir(repo, runID), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(filepath.Join(runDir(repo, runID), "state.json"), append(data, '\n'), 0o644)
}

// lockRunStateFile serializes state readers and writers across oz processes.
func lockRunStateFile(repo, runID string) (func() error, error) {
	dir := runDir(repo, runID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return flockLock(filepath.Join(dir, "state.json.lock"))
}

// latestWorkerForSave preserves newer heartbeat and terminal metadata from durable state.
func latestWorkerForSave(path, runID string, incoming *WorkerRuntimeState) *WorkerRuntimeState {
	data, err := os.ReadFile(path)
	if err != nil {
		return cloneWorkerRuntime(incoming)
	}
	var current State
	if err := json.Unmarshal(data, &current); err != nil || current.RunID != runID {
		return cloneWorkerRuntime(incoming)
	}
	return mergeWorkerRuntime(incoming, current.Worker)
}

// mergeWorkerRuntime keeps worker identity and timestamps monotonic across stale whole-state saves.
func mergeWorkerRuntime(incoming, current *WorkerRuntimeState) *WorkerRuntimeState {
	if incoming == nil {
		return cloneWorkerRuntime(current)
	}
	if current == nil {
		return cloneWorkerRuntime(incoming)
	}
	if !sameWorkerGeneration(incoming, current) {
		if timestampAfter(current.StartedAt, incoming.StartedAt) {
			return cloneWorkerRuntime(current)
		}
		return cloneWorkerRuntime(incoming)
	}
	merged := *incoming
	if timestampAfter(current.LastHeartbeatAt, merged.LastHeartbeatAt) {
		merged.LastHeartbeatAt = current.LastHeartbeatAt
	}
	if current.Exit != "" && (merged.Exit == "" || timestampAfter(current.FinishedAt, merged.FinishedAt)) {
		merged.FinishedAt = current.FinishedAt
		merged.Exit = current.Exit
		merged.Error = current.Error
	}
	if merged.LogPath == "" {
		merged.LogPath = current.LogPath
	}
	return &merged
}

// sameWorkerGeneration identifies one worker process incarnation.
func sameWorkerGeneration(left, right *WorkerRuntimeState) bool {
	return left != nil && right != nil &&
		left.PID == right.PID &&
		left.Hostname == right.Hostname &&
		left.StartedAt == right.StartedAt
}

// cloneWorkerRuntime copies optional worker metadata without retaining caller-owned pointers.
func cloneWorkerRuntime(worker *WorkerRuntimeState) *WorkerRuntimeState {
	if worker == nil {
		return nil
	}
	cloned := *worker
	return &cloned
}

// timestampAfter compares optional RFC3339 timestamps conservatively.
func timestampAfter(left, right string) bool {
	if left == "" {
		return false
	}
	if right == "" {
		return true
	}
	leftTime, leftErr := time.Parse(time.RFC3339Nano, left)
	rightTime, rightErr := time.Parse(time.RFC3339Nano, right)
	return leftErr == nil && rightErr == nil && leftTime.After(rightTime)
}

// validateRunID rejects run identifiers that could escape the runs directory.
func validateRunID(runID string) error {
	if runID == "" || runID == "." || runID == ".." || filepath.IsAbs(runID) || strings.Contains(runID, "..") || strings.ContainsAny(runID, `/\`) {
		return fmt.Errorf("invalid run_id %q", runID)
	}
	return nil
}
