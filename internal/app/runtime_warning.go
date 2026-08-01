// Package app records advisory workflow diagnostics without pausing delivery.
package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// recordRuntimeWarning writes non-blocking gate diagnostics into the run directory.
func recordRuntimeWarning(repo string, state State, kind, message string) {
	path := filepath.Join(runDir(repo, state.RunID), "runtime-warnings.json")
	var entries []map[string]string
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &entries)
	}
	entries = append(entries, map[string]string{"time": time.Now().UTC().Format(time.RFC3339Nano), "stage": state.Stage, "kind": kind, "message": message})
	if data, err := json.MarshalIndent(entries, "", "  "); err == nil {
		_ = os.WriteFile(path, append(data, '\n'), 0o644)
	}
}
