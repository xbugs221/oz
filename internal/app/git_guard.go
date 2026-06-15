// Package app snapshots git state and guards sealed runs from unrelated source edits.
package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type gitSnapshotGuard struct {
	Blocked bool
	Paths   []string
	Allowed []string
}

// gitSnapshot captures HEAD and porcelain status for intervention checks.
func gitSnapshot(repo string) (string, string, error) {
	gitPath, err := resolveCommand("git")
	if err != nil {
		return "", "", err
	}
	headCmd := commandContext(context.Background(), gitPath, "rev-parse", "--verify", "HEAD^{commit}")
	headCmd.Dir = repo
	head, err := headCmd.CombinedOutput()
	if err != nil {
		headErr := strings.TrimSpace(string(head))
		if isUnbornBranch(repo, gitPath) {
			return "", "", fmt.Errorf(errNoInitialCommit)
		}
		if headErr != "" {
			return "", "", fmt.Errorf("git rev-parse --verify HEAD 失败：%s", headErr)
		}
		return "", "", err
	}
	statusCmd := commandContext(context.Background(), gitPath, "-c", "core.quotePath=false", "status", "--porcelain")
	statusCmd.Dir = repo
	status, err := statusCmd.CombinedOutput()
	if err != nil {
		statusErr := strings.TrimSpace(string(status))
		if statusErr != "" {
			return "", "", fmt.Errorf("git status --porcelain 失败：%s", statusErr)
		}
		return "", "", err
	}
	return strings.TrimSpace(string(head)), filterRuntimeStatus(string(status)), nil
}

// isUnbornBranch confirms HEAD is a symbolic branch that has not received a commit yet.
func isUnbornBranch(repo, gitPath string) bool {
	cmd := commandContext(context.Background(), gitPath, "symbolic-ref", "-q", "HEAD")
	cmd.Dir = repo
	out, err := cmd.Output()
	ref := strings.TrimSpace(string(out))
	if err != nil || !strings.HasPrefix(ref, "refs/heads/") {
		return false
	}
	verifyCmd := commandContext(context.Background(), gitPath, "show-ref", "--verify", "--quiet", ref)
	verifyCmd.Dir = repo
	return verifyCmd.Run() != nil
}

// filterRuntimeStatus removes workflow-owned runtime paths from git status snapshots.
func filterRuntimeStatus(status string) string {
	var kept []string
	for _, line := range strings.Split(status, "\n") {
		if line == "" {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if isRuntimeGitPath(path) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// Detail formats the paths that explain a git snapshot guard decision.
func (guard gitSnapshotGuard) Detail() string {
	if len(guard.Paths) == 0 {
		return "HEAD 变化"
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

// classifyGitSnapshotChange separates new unrelated demand proposal edits from current-run writes.
func classifyGitSnapshotChange(repo, changeName, beforeHead, beforeDiff, afterHead, afterDiff string) (gitSnapshotGuard, error) {
	return classifyGitSnapshotChangeWithAllowed(repo, changeName, beforeHead, beforeDiff, afterHead, afterDiff, nil)
}

// classifyGitSnapshotChangeWithAllowed allows scoped runtime artifact writes in addition to unrelated proposals.
func classifyGitSnapshotChangeWithAllowed(repo, changeName, beforeHead, beforeDiff, afterHead, afterDiff string, allowedDirs []string) (gitSnapshotGuard, error) {
	paths := changedStatusPaths(beforeDiff, afterDiff)
	if beforeHead != afterHead {
		commitPaths, err := committedPaths(repo, beforeHead, afterHead)
		if err != nil {
			return gitSnapshotGuard{Blocked: true}, err
		}
		paths = append(paths, commitPaths...)
	}
	var blocked []string
	var allowed []string
	allowedPrefixes := gitRelativeAllowedPrefixes(repo, allowedDirs)
	for _, path := range uniqueSortedPaths(paths) {
		if isUnrelatedChangePath(path, changeName) || isAllowedGitPath(path, allowedPrefixes) {
			allowed = append(allowed, path)
			continue
		}
		blocked = append(blocked, path)
	}
	return gitSnapshotGuard{Blocked: len(blocked) > 0, Paths: blocked, Allowed: allowed}, nil
}

// gitChangeContentSnapshot captures the actual repository change content, ignoring index-only churn.
func gitChangeContentSnapshot(repo string) (string, error) {
	gitPath, err := resolveCommand("git")
	if err != nil {
		return "", err
	}
	diffCmd := commandContext(
		context.Background(),
		gitPath,
		"-c", "core.quotePath=false",
		"diff", "--no-ext-diff", "--binary", "HEAD", "--",
		".",
		":(exclude).wo",
		":(exclude).wo/**",
		":(exclude)test-results",
		":(exclude)test-results/**",
	)
	diffCmd.Dir = repo
	diff, err := diffCmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(diff))
		if detail != "" {
			return "", fmt.Errorf("git diff HEAD 失败：%s", detail)
		}
		return "", err
	}
	untracked, err := untrackedContentSnapshot(repo, gitPath)
	if err != nil {
		return "", err
	}
	if len(untracked) == 0 {
		return string(diff), nil
	}
	return string(diff) + "\n-- oz untracked content --\n" + strings.Join(untracked, "\n"), nil
}

// classifyGitContentSnapshotChange reports paths whose actual repository change content changed.
func classifyGitContentSnapshotChange(repo, before, after string, allowedDirs []string) gitSnapshotGuard {
	paths := gitContentSnapshotChangedPaths(before, after)
	allowedPrefixes := gitRelativeAllowedPrefixes(repo, allowedDirs)
	var blocked []string
	var allowed []string
	for _, path := range uniqueSortedPaths(paths) {
		if isAllowedGitPath(path, allowedPrefixes) {
			allowed = append(allowed, path)
			continue
		}
		blocked = append(blocked, path)
	}
	if len(blocked) == 0 && before != after && len(paths) == 0 {
		return gitSnapshotGuard{Blocked: true}
	}
	return gitSnapshotGuard{Blocked: len(blocked) > 0, Paths: blocked, Allowed: allowed}
}

// untrackedContentSnapshot records untracked file contents without considering generated runtime paths.
func untrackedContentSnapshot(repo, gitPath string) ([]string, error) {
	cmd := commandContext(context.Background(), gitPath, "-c", "core.quotePath=false", "ls-files", "-z", "--others", "--exclude-standard", "--", ".")
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail != "" {
			return nil, fmt.Errorf("git ls-files --others 失败：%s", detail)
		}
		return nil, err
	}
	var entries []string
	for _, rawPath := range strings.Split(string(out), "\x00") {
		path := filepath.ToSlash(strings.TrimSpace(rawPath))
		if path == "" || isRuntimeGitPath(path) {
			continue
		}
		full := filepath.Join(repo, filepath.FromSlash(path))
		info, err := os.Lstat(full)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if info.IsDir() {
			continue
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		entries = append(entries, fmt.Sprintf("UNTRACKED\t%s\t%d:%s", path, len(data), hex.EncodeToString(sum[:])))
	}
	sort.Strings(entries)
	return entries, nil
}

// gitContentSnapshotChangedPaths extracts affected paths from two content snapshots.
func gitContentSnapshotChangedPaths(before, after string) []string {
	beforePaths := gitContentSnapshotPathMap(before)
	afterPaths := gitContentSnapshotPathMap(after)
	seen := map[string]bool{}
	var paths []string
	for path, beforeValue := range beforePaths {
		seen[path] = true
		if afterPaths[path] != beforeValue {
			paths = append(paths, path)
		}
	}
	for path, afterValue := range afterPaths {
		if seen[path] {
			continue
		}
		if beforePaths[path] != afterValue {
			paths = append(paths, path)
		}
	}
	return uniqueSortedPaths(paths)
}

// gitContentSnapshotPathMap indexes each path by its content-bearing snapshot lines.
func gitContentSnapshotPathMap(snapshot string) map[string]string {
	paths := map[string][]string{}
	var current string
	for _, line := range strings.Split(snapshot, "\n") {
		if path, ok := diffGitLinePath(line); ok {
			current = path
			paths[current] = append(paths[current], line)
			continue
		}
		if strings.HasPrefix(line, "UNTRACKED\t") {
			fields := strings.SplitN(line, "\t", 3)
			if len(fields) >= 2 {
				path := filepath.ToSlash(fields[1])
				paths[path] = append(paths[path], line)
			}
			current = ""
			continue
		}
		if current != "" {
			paths[current] = append(paths[current], line)
		}
	}
	out := map[string]string{}
	for path, lines := range paths {
		out[path] = strings.Join(lines, "\n")
	}
	return out
}

// diffGitLinePath extracts the destination path from a git diff header.
func diffGitLinePath(line string) (string, bool) {
	const prefix = "diff --git "
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(line, prefix)
	marker := " b/"
	idx := strings.LastIndex(rest, marker)
	if idx < 0 {
		return "", false
	}
	path := strings.TrimSpace(rest[idx+len(marker):])
	if path == "/dev/null" || path == "" {
		return "", false
	}
	return filepath.ToSlash(path), true
}

// gitRelativeAllowedPrefixes converts absolute artifact directories to git status path prefixes.
func gitRelativeAllowedPrefixes(repo string, dirs []string) []string {
	var prefixes []string
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		rel, err := filepath.Rel(repo, dir)
		if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			continue
		}
		prefix := strings.TrimPrefix(filepath.ToSlash(rel), "./")
		if prefix != "" && prefix != "." {
			prefixes = append(prefixes, strings.TrimSuffix(prefix, "/")+"/")
		}
	}
	return prefixes
}

// isRuntimeGitPath reports paths owned by workflow/test runtime output.
func isRuntimeGitPath(path string) bool {
	path = strings.TrimPrefix(filepath.ToSlash(path), "./")
	return path == ".wo" || strings.HasPrefix(path, ".wo/") || path == "test-results" || strings.HasPrefix(path, "test-results/")
}

// isAllowedGitPath checks whether a changed file is inside the current member artifact directory.
func isAllowedGitPath(path string, allowedPrefixes []string) bool {
	path = strings.TrimPrefix(filepath.ToSlash(path), "./")
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// changedStatusPaths returns paths whose porcelain status changed since the saved baseline.
func changedStatusPaths(before, after string) []string {
	beforeLines := statusLineByPath(before)
	afterLines := statusLineByPath(after)
	seen := map[string]bool{}
	var paths []string
	for path, line := range afterLines {
		if beforeLines[path] != line {
			paths = append(paths, path)
			seen[path] = true
		}
	}
	for path, line := range beforeLines {
		if seen[path] {
			continue
		}
		if afterLines[path] != line {
			paths = append(paths, path)
		}
	}
	return paths
}

// statusLineByPath indexes every normalized git status path by its full porcelain line.
func statusLineByPath(status string) map[string]string {
	lines := map[string]string{}
	for _, line := range strings.Split(status, "\n") {
		if line == "" {
			continue
		}
		for _, path := range porcelainLinePaths(line) {
			lines[path] = line
		}
	}
	return lines
}

// porcelainLinePaths extracts all business paths from one git status --porcelain line.
func porcelainLinePaths(line string) []string {
	if len(line) < 4 {
		return nil
	}
	path := strings.TrimSpace(line[3:])
	if renamed := strings.Split(path, " -> "); len(renamed) == 2 {
		return statusNamePaths(strings.Join(renamed, "\n"))
	}
	return statusNamePaths(path)
}

// committedPaths returns every file path touched by commits between two saved HEADs.
func committedPaths(repo, beforeHead, afterHead string) ([]string, error) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, err
	}
	cmd := commandContext(context.Background(), gitPath, "-c", "core.quotePath=false", "diff", "--name-status", "--find-renames", beforeHead, afterHead)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail != "" {
			return nil, fmt.Errorf("git diff --name-status 失败：%s", detail)
		}
		return nil, err
	}
	return statusNamePaths(string(out)), nil
}

// statusNamePaths normalizes paths from newline or tab separated git status output.
func statusNamePaths(output string) []string {
	var paths []string
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Split(strings.TrimSpace(line), "\t")
		if len(fields) > 1 {
			fields = fields[1:]
		}
		for _, field := range fields {
			path := strings.TrimSpace(field)
			if path != "" {
				paths = append(paths, filepath.ToSlash(path))
			}
		}
	}
	return paths
}

// uniqueSortedPaths removes duplicate path entries for stable guard messages.
func uniqueSortedPaths(paths []string) []string {
	seen := map[string]bool{}
	var unique []string
	for _, path := range paths {
		path = filepath.ToSlash(strings.TrimSpace(path))
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		unique = append(unique, path)
	}
	sort.Strings(unique)
	return unique
}

// isUnrelatedChangePath returns true only for docs/changes entries outside the active change.
func isUnrelatedChangePath(path, changeName string) bool {
	path = strings.TrimPrefix(filepath.ToSlash(path), "./")
	const prefix = "docs/changes/"
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	rest := strings.TrimPrefix(path, prefix)
	if rest == "" || strings.HasPrefix(rest, "archive/") || strings.HasPrefix(rest, ".") {
		return false
	}
	change := rest
	if slash := strings.IndexByte(rest, '/'); slash >= 0 {
		change = rest[:slash]
	}
	return change != "" && change != changeName
}
