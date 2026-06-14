// Package app provides cross-platform filesystem and process helpers for oz flow.
package app

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
)

// repoRelPath returns a stable slash-separated path for state.json fields.
func repoRelPath(repo, absolutePath string) (string, error) {
	rel, err := filepath.Rel(repo, absolutePath)
	if err != nil {
		return "", fmt.Errorf("生成仓库相对路径失败：%w", err)
	}
	return filepath.ToSlash(rel), nil
}

// repoAbsPath converts a slash-separated state path into a native filesystem path.
func repoAbsPath(repo, statePath string) string {
	return filepath.Join(repo, filepath.FromSlash(statePath))
}

// resolveCommand finds a command through the host PATH, including Windows PATHEXT shims.
func resolveCommand(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("找不到 %s 可执行文件：%w", name, err)
	}
	return path, nil
}

// ensureBaseWorkflowCommands verifies external tools every workflow path needs.
func ensureBaseWorkflowCommands() error {
	if _, err := resolveCommand("git"); err != nil {
		return err
	}
	return nil
}

// commandContext builds a direct child process without invoking a shell.
func commandContext(ctx context.Context, path string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, path, args...)
}
