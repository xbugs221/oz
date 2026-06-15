// Package app enforces subagent read-only filesystem boundaries.
package app

import (
	"context"
	"crypto/sha1"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type subagentBoundaryRepair struct {
	Reverted []string
}

// checkSubagentReadOnlyBoundary reverts illegal yolo-helper writes while preserving the member artifact.
func (e *Engine) checkSubagentReadOnlyBoundary(state State, member ParallelMemberConfig, attempt int, artifactPath, beforeHead, beforeDiff, beforeContent string, beforeRunFiles map[string]string) (subagentBoundaryRepair, error) {
	afterContent, err := gitChangeContentSnapshot(e.Repo)
	if err != nil {
		return subagentBoundaryRepair{}, err
	}
	runGuard, err := classifyRunArtifactChanges(runDir(e.Repo, state.RunID), beforeRunFiles, filepath.Dir(artifactPath))
	if err != nil {
		return subagentBoundaryRepair{}, e.failNodeState(state, err)
	}
	if beforeContent == afterContent && !runGuard.Blocked {
		return subagentBoundaryRepair{}, nil
	}
	guard := classifyGitContentSnapshotChange(e.Repo, beforeContent, afterContent, []string{filepath.Dir(artifactPath)})
	if guard.Blocked || runGuard.Blocked {
		repair, repairErr := e.revertSubagentBoundaryChanges(state, beforeHead, beforeDiff, beforeRunFiles, guard, runGuard)
		if repairErr != nil {
			detail := guard.Detail()
			if runGuard.Blocked {
				detail = runGuard.Detail()
				if guard.Blocked {
					detail = guard.Detail() + "; " + runGuard.Detail()
				}
			}
			return repair, e.failNodeState(state, fmt.Errorf("subagent %s 第 %d 次尝试破坏只读边界且自动撤回失败：检测到当前 run 相关路径或源码变化（%s），artifact=%s：%v", member.Name, attempt, detail, artifactPath, repairErr))
		}
		return repair, nil
	}
	return subagentBoundaryRepair{}, nil
}

// revertSubagentBoundaryChanges removes only illegal deltas created during this helper attempt.
func (e *Engine) revertSubagentBoundaryChanges(state State, beforeHead, beforeDiff string, beforeRunFiles map[string]string, guard gitSnapshotGuard, runGuard runArtifactGuard) (subagentBoundaryRepair, error) {
	var repair subagentBoundaryRepair
	if guard.Blocked {
		reverted, err := revertGitBoundaryChanges(e.Repo, beforeHead, beforeDiff, guard.Paths)
		if err != nil {
			return repair, err
		}
		repair.Reverted = append(repair.Reverted, reverted...)
	}
	if runGuard.Blocked {
		reverted, err := revertRunArtifactBoundaryChanges(runDir(e.Repo, state.RunID), beforeRunFiles, runGuard.Paths)
		if err != nil {
			return repair, err
		}
		repair.Reverted = append(repair.Reverted, reverted...)
	}
	return repair, nil
}

// revertGitBoundaryChanges restores clean-baseline source deltas and refuses ambiguous dirty paths.
func revertGitBoundaryChanges(repo, beforeHead, beforeDiff string, paths []string) ([]string, error) {
	if strings.TrimSpace(beforeHead) == "" {
		return nil, fmt.Errorf("缺少 subagent 前置 HEAD，不能安全撤回源码变化")
	}
	beforeLines := statusLineByPath(beforeDiff)
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, err
	}
	headCmd := commandContext(context.Background(), gitPath, "rev-parse", "--verify", "HEAD^{commit}")
	headCmd.Dir = repo
	headOut, err := headCmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	if currentHead := strings.TrimSpace(string(headOut)); currentHead != beforeHead {
		return nil, fmt.Errorf("HEAD 已从 %s 变化为 %s，不能自动撤回 subagent commit", shortCommit(beforeHead), shortCommit(currentHead))
	}
	var reverted []string
	for _, path := range uniqueSortedPaths(paths) {
		if beforeLines[path] != "" {
			return reverted, fmt.Errorf("路径 %s 在 subagent 运行前已有未提交变化，不能自动撤回", path)
		}
		status, err := gitStatusLineForPath(repo, gitPath, path)
		if err != nil {
			return reverted, err
		}
		if strings.HasPrefix(status, "?? ") {
			if err := os.RemoveAll(filepath.Join(repo, filepath.FromSlash(path))); err != nil {
				return reverted, err
			}
			reverted = append(reverted, path)
			continue
		}
		cmd := commandContext(context.Background(), gitPath, "restore", "--worktree", "--staged", "--", path)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			detail := strings.TrimSpace(string(out))
			if detail != "" {
				return reverted, fmt.Errorf("git restore %s 失败：%s", path, detail)
			}
			return reverted, err
		}
		reverted = append(reverted, path)
	}
	return reverted, nil
}

// gitStatusLineForPath returns current porcelain status for one path.
func gitStatusLineForPath(repo, gitPath, path string) (string, error) {
	cmd := commandContext(context.Background(), gitPath, "-c", "core.quotePath=false", "status", "--porcelain", "--", path)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail != "" {
			return "", fmt.Errorf("git status %s 失败：%s", path, detail)
		}
		return "", err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			return line, nil
		}
	}
	return "", nil
}

// revertRunArtifactBoundaryChanges deletes only newly-created illegal run artifacts.
func revertRunArtifactBoundaryChanges(root string, before map[string]string, paths []string) ([]string, error) {
	var reverted []string
	for _, path := range uniqueSortedPaths(paths) {
		if before[path] != "" {
			return reverted, fmt.Errorf("run artifact %s 在 subagent 运行前已存在，不能自动撤回", path)
		}
		if err := os.RemoveAll(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			return reverted, err
		}
		reverted = append(reverted, path)
	}
	return reverted, nil
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

// classifyRunArtifactChanges blocks run-local writes outside subagent-owned artifacts.
func classifyRunArtifactChanges(root string, before map[string]string, allowedDir string) (runArtifactGuard, error) {
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
			// state.json is owned by the workflow engine; concurrent progress persistence
			// must not be attributed to a read-only helper process.
			continue
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

// shortCommit returns a readable prefix for full git commit ids.
func shortCommit(commit string) string {
	commit = strings.TrimSpace(commit)
	if len(commit) <= 12 {
		return commit
	}
	return commit[:12]
}
