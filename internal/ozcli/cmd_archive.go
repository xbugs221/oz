// Package ozcli implements standalone oz change archiving.
package ozcli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (c *cli) archiveCmd(args []string) error {
	// archiveCmd performs deterministic file moves; agents merge archived specs afterward.
	if hasHelp(args) {
		fmt.Fprintln(c.out, "用法：oz archive <change> --yes")
		return nil
	}
	if len(args) == 0 {
		return errors.New("用法：oz archive <change> --yes")
	}
	change := args[0]
	if !hasArg(args[1:], "--yes") {
		return errors.New("归档必须显式传入 --yes")
	}
	root, err := stateRoot()
	if err != nil {
		return err
	}
	result := validateChange(root, change)
	if !result.Valid {
		return fmt.Errorf("%s: %s", change, strings.Join(result.Errors, "; "))
	}
	if err := ensureTasksDone(filepath.Join(root, "changes", change, "task.md")); err != nil {
		return err
	}
	date := c.now().Format("2006-01-02")
	changeDir := filepath.Join(root, "changes", change)
	archiveDir := filepath.Join(root, "changes", "archive", date+"-"+change)
	if _, err := os.Stat(archiveDir); err == nil {
		return fmt.Errorf("归档目标已存在：%s", archiveDir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(archiveDir), 0o755); err != nil {
		return err
	}
	if err := os.Rename(changeDir, archiveDir); err != nil {
		return err
	}
	fmt.Fprintf(c.out, "已归档到 %s\n", archiveDir)
	return nil
}

func ensureTasksDone(path string) error {
	// ensureTasksDone prevents archiving unfinished task lists.
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [ ]") {
			return errors.New("task.md 包含未完成任务")
		}
	}
	return nil
}
