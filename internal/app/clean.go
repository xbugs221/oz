// Package app implements wo clean, which removes failed and abnormal runtime state.
package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// CleanResult holds the summary counts from a clean operation.
type CleanResult struct {
	CleanedRuns         int
	CleanedBatches      int
	SkippedRunning      int
	CleanedAgentRecords int
}

// cleanableRunStatuses defines run statuses that wo clean considers garbage.
var cleanableRunStatuses = map[string]bool{
	statusFailed:            true,
	statusInterrupted:       true,
	statusBlocked:           true,
	statusValidationBlocked: true,
	statusAborted:           true,
	"aborted":               true,
}

// cleanableBatchStatuses defines batch statuses that wo clean considers garbage.
var cleanableBatchStatuses = map[string]bool{
	batchStatusFailed:  true,
	batchStatusAborted: true,
}

// CleanRuntimeState scans runs and batches for the given repository and removes
// failed, interrupted, blocked, aborted, and corrupted state, respecting active locks.
func CleanRuntimeState(repo string) (CleanResult, error) {
	var result CleanResult
	cleanedRunIDs := map[string]bool{}
	cleanableSessions := map[string]bool{}
	protectedSessions := map[string]bool{}

	// Phase 0: pre-scan batches to build a protected-run-IDs set.
	// If any cleanable batch references an active-locked run, all runs referenced
	// by that batch are protected from independent deletion in Phase 1.
	protectedRunIDs := buildProtectedRunIDs(repo)

	// Phase 1: scan and clean runs first, collect cleaned run IDs.
	runRoot, err := runsRoot(repo)
	if err != nil {
		return result, err
	}
	runEntries, err := os.ReadDir(runRoot)
	if os.IsNotExist(err) {
		runEntries = nil
	} else if err != nil {
		return result, err
	}
	for _, entry := range runEntries {
		if !entry.IsDir() {
			continue
		}
		runID := entry.Name()
		dir := runDir(repo, runID)
		state, err := loadCleanRunState(repo, runID)
		if err != nil {
			// Missing or corrupt state.json; preserve if protected by a batch with active run.
			if protectedRunIDs[runID] {
				continue
			}
			if err := os.RemoveAll(dir); err != nil {
				return result, err
			}
			result.CleanedRuns++
			cleanedRunIDs[runID] = true
			continue
		}
		// Preserve done and archived runs.
		if state.Status == statusDone || state.Status == statusArchived {
			collectAgentSessions(state, protectedSessions)
			continue
		}
		// Detect actively-locked running runs, count as skipped, preserve directory.
		if state.Status == statusRunning {
			collectAgentSessions(state, protectedSessions)
			status, lockErr := lockFileStatus(repo, runID, runtime.GOOS)
			if lockErr == nil && status == lockStatusActive {
				result.SkippedRunning++
			}
			continue
		}
		// Only clean known terminal/failure statuses.
		if !cleanableRunStatuses[state.Status] {
			collectAgentSessions(state, protectedSessions)
			continue
		}
		// Check active lock (per-run).  Counts individually even if the run
		// is also protected by a batch in Phase 0.
		status, lockErr := lockFileStatus(repo, runID, runtime.GOOS)
		if lockErr == nil && status == lockStatusActive {
			collectAgentSessions(state, protectedSessions)
			result.SkippedRunning++
			continue
		}
		// Skip runs protected by a batch that has an active referenced run.
		// Such runs are NOT individually locked but must be preserved alongside
		// the protected batch (design.md: atomic batch preservation).
		if protectedRunIDs[runID] {
			collectAgentSessions(state, protectedSessions)
			continue
		}
		// Clean it.
		collectAgentSessions(state, cleanableSessions)
		if err := os.RemoveAll(dir); err != nil {
			return result, err
		}
		result.CleanedRuns++
		cleanedRunIDs[runID] = true
	}

	// Phase 2: scan and clean batches.
	batchRoot, err := batchesRoot(repo)
	if err != nil {
		return result, err
	}
	batchEntries, err := os.ReadDir(batchRoot)
	if os.IsNotExist(err) {
		batchEntries = nil
	} else if err != nil {
		return result, err
	}
	for _, entry := range batchEntries {
		if !entry.IsDir() {
			continue
		}
		batchID := entry.Name()
		batchDir := batchDir(repo, batchID)
		batch, err := loadCleanBatchState(repo, batchID)
		if err != nil {
			// Missing or corrupt batch state.json -> clean it.
			if err := os.RemoveAll(batchDir); err != nil {
				return result, err
			}
			result.CleanedBatches++
			continue
		}
		// Only clean failed or aborted batches.
		if !cleanableBatchStatuses[batch.Status] {
			continue
		}

		// Collect referenced runs: both failed_run_id and all non-empty run_ids values.
		refdRunIDs := batchReferencedRunIDs(batch)

		// Check if any referenced run has an active lock; if so, skip the whole batch.
		hasActiveRun := false
		for _, rid := range refdRunIDs {
			if cleanedRunIDs[rid] {
				// Already cleaned in phase 1 — not protected.
				continue
			}
			status, lerr := lockFileStatus(repo, rid, runtime.GOOS)
			if lerr == nil && status == lockStatusActive {
				hasActiveRun = true
				break
			}
		}
		if hasActiveRun {
			result.SkippedRunning++
			continue
		}

		// Delete referenced run dirs first (skip already cleaned, missing, or preserved runs).
		for _, rid := range refdRunIDs {
			if cleanedRunIDs[rid] {
				continue
			}
			// Reuse the same run status policy as Phase 1. Batch cleanup may remove
			// corrupt referenced runs, but valid running/done/archived/unknown runs
			// are still live or retained history and must protect their sessions.
			runState, err := loadCleanRunState(repo, rid)
			if err == nil {
				if !cleanableRunStatuses[runState.Status] {
					collectAgentSessions(runState, protectedSessions)
					continue
				}
			}
			rd := runDir(repo, rid)
			// Only count the run if its directory actually existed before removal.
			// Non-existent directories (e.g. run_ids referencing never-created runs)
			// are treated as already absent per design.md.
			if _, statErr := os.Stat(rd); statErr == nil {
				if err == nil {
					collectAgentSessions(runState, cleanableSessions)
				}
				if err := os.RemoveAll(rd); err != nil {
					return result, err
				}
				result.CleanedRuns++
			}
			cleanedRunIDs[rid] = true
		}

		// Delete the batch dir.
		if err := os.RemoveAll(batchDir); err != nil {
			return result, err
		}
		result.CleanedBatches++
	}

	result.CleanedAgentRecords = cleanAgentSessionRecords(cleanableSessions, protectedSessions)
	return result, nil
}

// collectAgentSessions records Codex/Pi child session IDs referenced by a run state.
func collectAgentSessions(state State, sessions map[string]bool) {
	for key, sessionID := range state.Sessions {
		if sessionID == "" {
			continue
		}
		if strings.HasPrefix(key, "codex:") || strings.HasPrefix(key, "pi:") {
			sessions[sessionID] = true
		}
	}
}

// cleanAgentSessionRecords removes external Codex/Pi records for sessions only
// referenced by runs that wo clean is deleting.
func cleanAgentSessionRecords(cleanableSessions, protectedSessions map[string]bool) int {
	targets := map[string]bool{}
	for sessionID := range cleanableSessions {
		if !protectedSessions[sessionID] {
			targets[sessionID] = true
		}
	}
	if len(targets) == 0 {
		return 0
	}
	cleaned := 0
	cleaned += cleanJSONLSessionFiles(codexSessionsRoot(), targets)
	cleaned += cleanJSONLSessionFiles(piSessionsRoot(), targets)
	cleaned += cleanPiSQLiteSessionRows(targets)
	return cleaned
}

// codexSessionsRoot returns the only Codex directory wo clean scans.
func codexSessionsRoot() string {
	if home := os.Getenv("CODEX_HOME"); home != "" {
		return filepath.Join(home, "sessions")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex", "sessions")
}

// piSessionsRoot returns the Pi JSONL session directory.
func piSessionsRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".pi", "agent", "sessions")
}

// piAgentRoot returns the Pi agent directory that may contain SQLite indexes.
func piAgentRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".pi", "agent")
}

// cleanJSONLSessionFiles deletes ordinary .jsonl files whose basename contains a full session ID.
func cleanJSONLSessionFiles(root string, sessionIDs map[string]bool) int {
	if root == "" {
		return 0
	}
	if _, err := os.Stat(root); err != nil {
		return 0
	}
	cleaned := 0
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		name := entry.Name()
		for sessionID := range sessionIDs {
			if strings.Contains(name, sessionID) {
				if os.Remove(path) == nil {
					cleaned++
				}
				break
			}
		}
		return nil
	})
	return cleaned
}

// cleanPiSQLiteSessionRows removes matching rows from known Pi SQLite schemas.
func cleanPiSQLiteSessionRows(sessionIDs map[string]bool) int {
	root := piAgentRoot()
	if root == "" {
		return 0
	}
	if _, err := os.Stat(root); err != nil {
		return 0
	}
	cleaned := 0
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !isSQLiteFile(path) {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		cleaned += cleanSQLiteFileWithPython(path, sessionIDs)
		return nil
	})
	return cleaned
}

// isSQLiteFile recognizes the conservative set of Pi database extensions.
func isSQLiteFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".sqlite", ".sqlite3", ".db":
		return true
	default:
		return false
	}
}

// cleanSQLiteFileWithPython uses optional python3 sqlite3 support without adding
// a runtime Go SQLite dependency; failures are intentionally best-effort skips.
func cleanSQLiteFileWithPython(dbPath string, sessionIDs map[string]bool) int {
	python, err := exec.LookPath("python3")
	if err != nil {
		return 0
	}
	args := []string{python, "-", dbPath}
	for sessionID := range sessionIDs {
		args = append(args, sessionID)
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = strings.NewReader(piSQLiteCleanerScript)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return 0
	}
	var cleaned int
	if _, err := fmt.Fscanf(&stdout, "%d", &cleaned); err != nil {
		return 0
	}
	return cleaned
}

const piSQLiteCleanerScript = `
import sqlite3
import sys

db_path = sys.argv[1]
session_ids = sys.argv[2:]
known = {
    "sessions": ("id",),
    "messages": ("session_id",),
    "events": ("session_id",),
    "turns": ("session_id",),
}

cleaned = 0
try:
    conn = sqlite3.connect(db_path, timeout=0.1)
    try:
        rows = conn.execute("select name from sqlite_master where type = 'table'").fetchall()
        tables = {name for (name,) in rows}
        for table, columns in known.items():
            if table not in tables:
                continue
            table_columns = {row[1] for row in conn.execute(f"pragma table_info({table})")}
            for column in columns:
                if column not in table_columns:
                    continue
                table_cleaned = False
                for session_id in session_ids:
                    before = conn.total_changes
                    conn.execute(f"delete from {table} where {column} = ?", (session_id,))
                    if conn.total_changes > before:
                        table_cleaned = True
                if table_cleaned:
                    cleaned += 1
                break
        conn.commit()
    finally:
        conn.close()
except Exception:
    cleaned = 0

print(cleaned)
`

// buildProtectedRunIDs pre-scans batches to find run IDs that must be preserved
// because they belong to a cleanable batch that references an active-locked run.
// Design.md: if any referenced run has an active lock, skip the whole batch.
func buildProtectedRunIDs(repo string) map[string]bool {
	protected := map[string]bool{}
	batchRoot, err := batchesRoot(repo)
	if err != nil {
		return protected
	}
	batchEntries, err := os.ReadDir(batchRoot)
	if os.IsNotExist(err) {
		return protected
	} else if err != nil {
		return protected
	}
	for _, entry := range batchEntries {
		if !entry.IsDir() {
			continue
		}
		batchID := entry.Name()
		batch, err := loadCleanBatchState(repo, batchID)
		if err != nil {
			continue // corrupt batch — will be cleaned in Phase 2, needs no protection
		}
		if !cleanableBatchStatuses[batch.Status] {
			continue
		}
		refdRunIDs := batchReferencedRunIDs(batch)
		hasActive := false
		for _, rid := range refdRunIDs {
			status, lockErr := lockFileStatus(repo, rid, runtime.GOOS)
			if lockErr == nil && status == lockStatusActive {
				hasActive = true
				break
			}
		}
		if hasActive {
			for _, rid := range refdRunIDs {
				protected[rid] = true
			}
		}
	}
	return protected
}

// loadCleanRunState tries to parse state.json from a run directory. Returns error
// if the file is missing or corrupt.
func loadCleanRunState(repo, runID string) (State, error) {
	data, err := os.ReadFile(filepath.Join(runDir(repo, runID), "state.json"))
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, err
	}
	return state, nil
}

// loadCleanBatchState tries to parse state.json from a batch directory.
func loadCleanBatchState(repo, batchID string) (BatchState, error) {
	data, err := os.ReadFile(filepath.Join(batchDir(repo, batchID), "state.json"))
	if err != nil {
		return BatchState{}, err
	}
	var batch BatchState
	if err := json.Unmarshal(data, &batch); err != nil {
		return BatchState{}, err
	}
	return batch, nil
}

// batchReferencedRunIDs collects non-empty run IDs referenced by a batch,
// including both the failed_run_id and all values in the run_ids map.
func batchReferencedRunIDs(batch BatchState) []string {
	seen := map[string]bool{}
	var ids []string
	add := func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		ids = append(ids, id)
	}
	add(batch.FailedRunID)
	for _, rid := range batch.RunIDs {
		add(rid)
	}
	return ids
}

// formatCleanResult builds human-readable Chinese output for wo clean.
func formatCleanResult(result CleanResult, repo string) string {
	if result.CleanedBatches == 0 && result.CleanedRuns == 0 {
		var lines []string
		if result.SkippedRunning > 0 {
			lines = append(lines, fmt.Sprintf("已跳过 %d 个仍在运行的任务", result.SkippedRunning))
			lines = append(lines, fmt.Sprintf("范围: 当前项目 %s", repo))
			lines = append(lines, "该操作仅删除 wo 历史记录，不回滚代码改动")
			return fmt.Sprintf("%s\n", joinLines(lines))
		}
		lines = append(lines, "没有需要清理的失败或异常运行态")
		lines = append(lines, "该操作仅检查当前项目 wo 历史记录，不回滚代码改动")
		return fmt.Sprintf("%s\n", joinLines(lines))
	}
	var lines []string
	lines = append(lines, fmt.Sprintf("已清理 %d 个批量任务、%d 个工作流、%d 个 agent 子会话记录", result.CleanedBatches, result.CleanedRuns, result.CleanedAgentRecords))
	if result.SkippedRunning > 0 {
		lines = append(lines, fmt.Sprintf("已跳过 %d 个仍在运行的任务", result.SkippedRunning))
	}
	lines = append(lines, fmt.Sprintf("范围: 当前项目 %s", repo))
	lines = append(lines, "该操作仅删除 wo 历史记录，不回滚代码改动")
	return fmt.Sprintf("%s\n", joinLines(lines))
}

func joinLines(lines []string) string {
	result := ""
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}

// runClean executes the wo clean command and writes human-readable output.
func runClean(stdout io.Writer, repo string) error {
	result, err := CleanRuntimeState(repo)
	if err != nil {
		return err
	}
	fmt.Fprint(stdout, formatCleanResult(result, repo))
	return nil
}
