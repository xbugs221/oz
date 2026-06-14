// Package app stores sealed workflow run state in per-run JSON files.
package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var stateFileMu sync.Mutex

// saveState writes durable workflow state as pretty JSON.
func saveState(repo string, state State) error {
	stateFileMu.Lock()
	defer stateFileMu.Unlock()
	if err := validateRunID(state.RunID); err != nil {
		return err
	}
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

// writeStateFileLocked writes state.json for the explicit run while the caller holds stateFileMu.
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
	return os.WriteFile(filepath.Join(runDir(repo, runID), "state.json"), append(data, '\n'), 0o644)
}

// validateRunID rejects run identifiers that could escape the runs directory.
func validateRunID(runID string) error {
	if runID == "" || runID == "." || runID == ".." || filepath.IsAbs(runID) || strings.Contains(runID, "..") || strings.ContainsAny(runID, `/\`) {
		return fmt.Errorf("invalid run_id %q", runID)
	}
	return nil
}
