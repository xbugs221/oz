// Package app enforces subagent read-only filesystem boundaries.
package app

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (e *Engine) checkSubagentReadOnlyBoundary(state State, member ParallelMemberConfig, attempt int, artifactPath, beforeHead, beforeDiff string, beforeRunFiles map[string]string, beforeState State, sessionKey string) error {
	afterHead, afterDiff, err := gitSnapshot(e.Repo)
	if err != nil {
		return err
	}
	runGuard, err := classifyRunArtifactChanges(runDir(e.Repo, state.RunID), beforeRunFiles, filepath.Dir(artifactPath), beforeState, sessionKey)
	if err != nil {
		return e.failNodeState(state, err)
	}
	if beforeHead == afterHead && beforeDiff == afterDiff && !runGuard.Blocked {
		return nil
	}
	guard, err := classifyGitSnapshotChangeWithAllowed(e.Repo, state.ChangeName, beforeHead, beforeDiff, afterHead, afterDiff, []string{filepath.Dir(artifactPath)})
	if err != nil {
		return e.failNodeState(state, err)
	}
	if guard.Blocked || runGuard.Blocked {
		detail := guard.Detail()
		if runGuard.Blocked {
			detail = runGuard.Detail()
			if guard.Blocked {
				detail = guard.Detail() + "; " + runGuard.Detail()
			}
		}
		return e.failNodeState(state, fmt.Errorf("subagent %s 第 %d 次尝试破坏只读边界：检测到当前 run 相关路径或源码变化（%s），artifact=%s", member.Name, attempt, detail, artifactPath))
	}
	return nil
}

type runArtifactGuard struct {
	Blocked bool
	Paths   []string
}

// Detail formats run artifact paths that explain a filesystem boundary decision.
func (guard runArtifactGuard) Detail() string {
	if len(guard.Paths) == 0 {
		return "run artifact 变化"
	}
	limit := len(guard.Paths)
	if limit > 5 {
		limit = 5
	}
	detail := strings.Join(guard.Paths[:limit], ", ")
	if len(guard.Paths) > limit {
		detail += fmt.Sprintf(" 等 %d 个路径", len(guard.Paths))
	}
	return detail
}

// runArtifactFileSnapshot records current run files so repo-external artifacts stay guarded.
func runArtifactFileSnapshot(root string) (map[string]string, error) {
	files := map[string]string{}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return files, nil
	} else if err != nil {
		return nil, err
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = fmt.Sprintf("%d:%x", info.Size(), sha1.Sum(data))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

// classifyRunArtifactChanges blocks run-local writes outside subagent-owned artifact/progress paths.
func classifyRunArtifactChanges(root string, before map[string]string, allowedDir string, beforeState State, sessionKey string) (runArtifactGuard, error) {
	after, err := runArtifactFileSnapshot(root)
	if err != nil {
		return runArtifactGuard{}, err
	}
	allowedRel, err := filepath.Rel(root, allowedDir)
	if err != nil {
		allowedRel = ""
	}
	allowedRel = strings.TrimSuffix(filepath.ToSlash(allowedRel), "/")
	var blocked []string
	for _, path := range changedRunArtifactPaths(before, after) {
		if allowedRel != "" && allowedRel != "." && (path == allowedRel || strings.HasPrefix(path, allowedRel+"/")) {
			continue
		}
		if isWritableParallelMemberArtifact(path, after) {
			continue
		}
		if path == "state.json" {
			ok, err := stateJSONOnlySubagentProgressChange(root, beforeState, sessionKey)
			if err != nil {
				return runArtifactGuard{}, err
			}
			if ok {
				continue
			}
		}
		blocked = append(blocked, path)
	}
	return runArtifactGuard{Blocked: len(blocked) > 0, Paths: blocked}, nil
}

// isWritableParallelMemberArtifact allows sibling helpers to create or rewrite their own member.json concurrently.
func isWritableParallelMemberArtifact(path string, after map[string]string) bool {
	if after[path] == "" {
		return false
	}
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) < 4 || parts[0] != "parallel-members" {
		return false
	}
	return parts[len(parts)-1] == "member.json" && strings.HasSuffix(parts[len(parts)-2], ".artifact")
}

// stateJSONOnlySubagentProgressChange allows framework-owned subagent progress persistence.
func stateJSONOnlySubagentProgressChange(root string, before State, sessionKey string) (bool, error) {
	if strings.TrimSpace(sessionKey) == "" {
		return false, nil
	}
	data, err := os.ReadFile(filepath.Join(root, "state.json"))
	if err != nil {
		return false, err
	}
	var after State
	if err := json.Unmarshal(data, &after); err != nil {
		return false, err
	}
	if after.Sessions == nil {
		return false, nil
	}
	if !subagentSessionChangesAllowed(before.Sessions, after.Sessions) {
		return false, nil
	}
	if !subagentDAGNodeChangesAllowed(before.Workflow, before.DAGNodes, after.DAGNodes) {
		return false, nil
	}
	normalized := after
	normalized.Sessions = copyStringMap(before.Sessions)
	normalized.DAGNodes = copyDAGNodeMap(before.DAGNodes)
	normalized.Processes = append([]ProcessState(nil), before.Processes...)
	beforeData, err := json.Marshal(before)
	if err != nil {
		return false, err
	}
	normalizedData, err := json.Marshal(normalized)
	if err != nil {
		return false, err
	}
	return bytes.Equal(beforeData, normalizedData), nil
}

// copyStringMap duplicates state maps before normalizing allowed deltas.
func copyStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	copied := make(map[string]string, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}

// copyDAGNodeMap duplicates DAG node state before normalizing framework progress deltas.
func copyDAGNodeMap(values map[string]DAGNodeState) map[string]DAGNodeState {
	if values == nil {
		return nil
	}
	copied := make(map[string]DAGNodeState, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}

// subagentSessionChangesAllowed limits state.json deltas to subagent session additions or updates.
func subagentSessionChangesAllowed(before, after map[string]string) bool {
	for key, value := range before {
		if after[key] != value {
			return false
		}
	}
	for key, value := range after {
		if before != nil && before[key] == value {
			continue
		}
		if value == "" || !isSubagentSessionKey(key) {
			return false
		}
	}
	return true
}

// isSubagentSessionKey reports whether a persisted session belongs to a helper member.
func isSubagentSessionKey(key string) bool {
	_, role, ok := strings.Cut(key, ":")
	return ok && strings.HasPrefix(role, "subagent:")
}

// subagentDAGNodeChangesAllowed limits state.json DAG progress deltas to configured subagent nodes.
func subagentDAGNodeChangesAllowed(workflow WorkflowConfig, before, after map[string]DAGNodeState) bool {
	for key, value := range before {
		afterValue, ok := after[key]
		if !ok {
			return false
		}
		if afterValue != value && !workflowSubagentNodeID(workflow, key) {
			return false
		}
	}
	for key := range after {
		if _, ok := before[key]; ok {
			continue
		}
		if !workflowSubagentNodeID(workflow, key) {
			return false
		}
	}
	return true
}

// workflowSubagentNodeID reports whether a DAG node id belongs to a configured helper member.
func workflowSubagentNodeID(workflow WorkflowConfig, id string) bool {
	spec := BuildWorkflowSpec("", workflow)
	for _, node := range spec.Nodes {
		if node.ID == id && node.Type == "subagent" {
			return true
		}
	}
	return false
}

// changedRunArtifactPaths returns files whose content appeared, disappeared, or changed.
func changedRunArtifactPaths(before, after map[string]string) []string {
	seen := map[string]bool{}
	var changed []string
	for path, beforeSig := range before {
		seen[path] = true
		if after[path] != beforeSig {
			changed = append(changed, path)
		}
	}
	for path := range after {
		if !seen[path] {
			changed = append(changed, path)
		}
	}
	return uniqueSortedPaths(changed)
}

func readOnlyBoundaryDetail(beforeHead, beforeDiff, afterHead, afterDiff string) string {
	var parts []string
	if beforeHead != afterHead {
		parts = append(parts, fmt.Sprintf("HEAD %s -> %s", shortCommit(beforeHead), shortCommit(afterHead)))
	}
	if diff := statusDeltaSummary(beforeDiff, afterDiff); diff != "" {
		parts = append(parts, diff)
	}
	if len(parts) == 0 {
		return "worktree changed"
	}
	return strings.Join(parts, "；")
}

// statusDeltaSummary returns added and removed porcelain status lines.
func statusDeltaSummary(before, after string) string {
	added, removed := statusDelta(before, after)
	var parts []string
	if len(added) > 0 {
		parts = append(parts, "新增/变更："+strings.Join(limitStatusLines(added), " | "))
	}
	if len(removed) > 0 {
		parts = append(parts, "消失："+strings.Join(limitStatusLines(removed), " | "))
	}
	return strings.Join(parts, "；")
}

// statusDelta compares git porcelain status strings while preserving display order.
func statusDelta(before, after string) ([]string, []string) {
	beforeLines := statusLines(before)
	afterLines := statusLines(after)
	beforeSet := map[string]bool{}
	for _, line := range beforeLines {
		beforeSet[line] = true
	}
	afterSet := map[string]bool{}
	for _, line := range afterLines {
		afterSet[line] = true
	}
	var added []string
	for _, line := range afterLines {
		if !beforeSet[line] {
			added = append(added, line)
		}
	}
	var removed []string
	for _, line := range beforeLines {
		if !afterSet[line] {
			removed = append(removed, line)
		}
	}
	return added, removed
}

// statusLines splits a porcelain status snapshot into non-empty lines.
func statusLines(status string) []string {
	var lines []string
	for _, line := range strings.Split(status, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// limitStatusLines caps diagnostics so workflow errors stay readable.
func limitStatusLines(lines []string) []string {
	const maxLines = 8
	if len(lines) <= maxLines {
		return lines
	}
	limited := append([]string{}, lines[:maxLines]...)
	limited = append(limited, fmt.Sprintf("... 还有 %d 项", len(lines)-maxLines))
	return limited
}

// shortCommit returns a readable prefix for full git commit ids.
func shortCommit(commit string) string {
	commit = strings.TrimSpace(commit)
	if len(commit) <= 12 {
		return commit
	}
	return commit[:12]
}
