// Package ozcli implements standalone oz change archiving.
package ozcli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
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
	if err := rewriteArchivedChangeReferences(archiveDir, change, date+"-"+change); err != nil {
		// Restore the active proposal when its contract cannot be rewritten, so a
		// failed archive never leaves an unusable half-migrated change behind.
		_ = os.Rename(archiveDir, changeDir)
		return err
	}
	fmt.Fprintf(c.out, "已归档到 %s\n", archiveDir)
	return nil
}

func rewriteArchivedChangeReferences(archiveDir, change, archivedChange string) error {
	// rewriteArchivedChangeReferences updates every UTF-8 text reference after
	// tests move from docs/changes/<change> into the dated archive tree.
	oldPrefix := "docs/changes/" + change + "/"
	newPrefix := "docs/changes/archive/" + archivedChange + "/"
	testPrefix := newPrefix + "tests/"
	return filepath.WalkDir(archiveDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !utf8.Valid(data) {
			return nil
		}
		updated := strings.ReplaceAll(string(data), oldPrefix, newPrefix)
		if !strings.Contains(filepath.ToSlash(path), "/tests/") {
			updated = rewriteRelativeTestReferences(updated, testPrefix)
		}
		if updated == string(data) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(path, []byte(updated), info.Mode().Perm())
	})
}

var relativeTestsReference = regexp.MustCompile(`(^|[[:space:]\x60"'(\[：])tests/`)

func rewriteRelativeTestReferences(text, testPrefix string) string {
	// rewriteRelativeTestReferences expands proposal-local tests/ links in
	// archived documents while leaving long-term tests/specs/ links untouched.
	var out strings.Builder
	last := 0
	for _, match := range relativeTestsReference.FindAllStringIndex(text, -1) {
		start, end := match[0], match[1]
		out.WriteString(text[last:start])
		if strings.HasPrefix(text[end:], "specs/") {
			out.WriteString(text[start:end])
		} else {
			out.WriteString(text[start : end-len("tests/")])
			out.WriteString(testPrefix)
		}
		last = end
	}
	out.WriteString(text[last:])
	return out.String()
}

func ensureTasksDone(path string) error {
	// ensureTasksDone prevents archiving unfinished task lists.
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
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
