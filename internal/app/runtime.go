// Package app resolves durable runtime paths outside business repositories.
package app

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// runtimeRoot returns the user state directory that owns wo runtime state.
func runtimeRoot() (string, error) {
	return runtimeRootForGOOS(runtime.GOOS)
}

// runtimeRootForGOOS resolves the state root for tests and platform-specific defaults.
func runtimeRootForGOOS(goos string) (string, error) {
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "wo"), nil
	}
	if goos == "windows" {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			return filepath.Join(local, "wo"), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("解析用户主目录失败：%w", err)
		}
		return filepath.Join(home, ".local", "state", "wo"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("解析用户主目录失败：%w", err)
	}
	return filepath.Join(home, ".local", "state", "wo"), nil
}

// repoRuntimeDir returns the state directory reserved for one repository.
func repoRuntimeDir(repo string) (string, error) {
	root, err := runtimeRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "repos", repoKey(repo)), nil
}

// repoKey builds a stable readable key from a repository absolute path.
func repoKey(repo string) string {
	abs, err := filepath.Abs(repo)
	if err != nil {
		abs = repo
	}
	clean := filepath.Clean(abs)
	sum := sha1.Sum([]byte(clean))
	hash := hex.EncodeToString(sum[:])[:10]
	name := filepath.Base(clean)
	name = sanitizeRepoKeyName(name)
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "repo"
	}
	return name + "-" + hash
}

// sanitizeRepoKeyName keeps repo keys portable and easy to inspect.
func sanitizeRepoKeyName(name string) string {
	name = strings.ToLower(name)
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// runsRoot returns the directory containing all runs for one repository.
func runsRoot(repo string) (string, error) {
	base, err := repoRuntimeDir(repo)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "runs"), nil
}

// batchesRoot returns the directory containing all batches for one repository.
func batchesRoot(repo string) (string, error) {
	base, err := repoRuntimeDir(repo)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "batches"), nil
}

// runDir returns the filesystem path for one run.
func runDir(repo, runID string) string {
	root, err := runsRoot(repo)
	if err != nil {
		return filepath.Join(repo, ".wo", "runs", runID)
	}
	return filepath.Join(root, runID)
}

// acceptancePath returns the structured acceptance contract path for a change.
func acceptancePath(repo, changeName string) string {
	return filepath.Join(repo, "docs", "changes", changeName, "acceptance.json")
}

// batchDir returns the filesystem path for one batch.
func batchDir(repo, batchID string) string {
	root, err := batchesRoot(repo)
	if err != nil {
		return filepath.Join(repo, ".wo", "batches", batchID)
	}
	return filepath.Join(root, batchID)
}
