// Package ozcli implements standalone oz change inspection commands.
package ozcli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/xbugs221/oz/internal/acceptance"
)

func (c *cli) listCmd(args []string) error {
	// listCmd reports active changes under docs/changes.
	if hasHelp(args) {
		c.printListHelp()
		return nil
	}
	jsonOut := hasArg(args, "--json")
	if len(args) > 1 || (len(args) == 1 && !jsonOut) {
		return errors.New("用法：oz list [--json]")
	}
	root, err := stateRoot()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Join(root, "changes"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	names := []string{}
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "archive" {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	if jsonOut {
		changes := []map[string]string{}
		for _, name := range names {
			changes = append(changes, map[string]string{"name": name})
		}
		return writeJSON(c.out, map[string]any{"changes": changes})
	}
	for _, name := range names {
		fmt.Fprintln(c.out, name)
	}
	return nil
}

func (c *cli) printListHelp() {
	// printListHelp documents both the full list command and its short alias.
	fmt.Fprintln(c.out, "用法：")
	fmt.Fprintln(c.out, "  oz list [--json]")
	fmt.Fprintln(c.out, "  oz l [--json]")
}

func (c *cli) createCmd(args []string) error {
	// createCmd reports the next proposal number while agents still create artifacts via skills.
	if hasHelp(args) {
		fmt.Fprintln(c.out, "用法：oz create")
		return nil
	}
	if len(args) != 0 {
		return errors.New("用法：oz create")
	}
	root, err := stateRoot()
	if err != nil {
		return err
	}
	next, err := nextChangeNumber(root)
	if err != nil {
		return err
	}
	fmt.Fprintln(c.out, next)
	return nil
}

func (c *cli) statusCmd(args []string) error {
	// statusCmd reports fixed artifact presence and task progress for one active change.
	if hasHelp(args) {
		fmt.Fprintln(c.out, "用法：oz status <change> [--json]")
		return nil
	}
	jsonOut := hasArg(args, "--json")
	change := firstPositional(args)
	if change == "" {
		return errors.New("用法：oz status <change> [--json]")
	}
	if err := validateNumberedChange(change); err != nil {
		return err
	}
	root, err := stateRoot()
	if err != nil {
		return err
	}
	payload := statusPayload(root, change)
	if jsonOut {
		return writeJSON(c.out, payload)
	}
	fmt.Fprintf(c.out, "%s：%s\n", change, displayStatus(payload["status"].(string)))
	for _, artifact := range payload["artifacts"].([]map[string]any) {
		fmt.Fprintf(c.out, "- %s：%s\n", artifact["name"], displayStatus(artifact["status"].(string)))
	}
	return nil
}

func nextChangeNumber(root string) (int, error) {
	// nextChangeNumber scans proposal directory names without requiring agents to print them into context.
	maxNumber := 0
	changesDir := filepath.Join(root, "changes")
	entries, err := os.ReadDir(changesDir)
	if errors.Is(err, os.ErrNotExist) {
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if entry.Name() == "archive" {
			archivedMax, err := maxArchivedChangeNumber(filepath.Join(changesDir, entry.Name()))
			if err != nil {
				return 0, err
			}
			if archivedMax > maxNumber {
				maxNumber = archivedMax
			}
			continue
		}
		if number, ok := activeChangeNumber(entry.Name()); ok && number > maxNumber {
			maxNumber = number
		}
	}
	return maxNumber + 1, nil
}

func maxArchivedChangeNumber(archiveDir string) (int, error) {
	// maxArchivedChangeNumber reads dated archive directory names like 2026-05-11-53-需求.
	entries, err := os.ReadDir(archiveDir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	maxNumber := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if number, ok := archivedChangeNumber(entry.Name()); ok && number > maxNumber {
			maxNumber = number
		}
	}
	return maxNumber, nil
}

func activeChangeNumber(name string) (int, bool) {
	// activeChangeNumber extracts the numeric prefix from active change directories.
	matches := activeChangeNumberRe.FindStringSubmatch(name)
	if matches == nil {
		return 0, false
	}
	number, err := strconv.Atoi(matches[1])
	return number, err == nil
}

func archivedChangeNumber(name string) (int, bool) {
	// archivedChangeNumber extracts the proposal number from dated archive directories.
	matches := archivedChangeNumberRe.FindStringSubmatch(name)
	if matches == nil {
		return 0, false
	}
	number, err := strconv.Atoi(matches[1])
	return number, err == nil
}

func statusPayload(root, change string) map[string]any {
	// statusPayload summarizes fixed oz artifacts without dynamic workflow configuration.
	changeDir := filepath.Join(root, "changes", change)
	artifacts := []map[string]any{}
	for _, name := range changeArtifactNames() {
		path := filepath.Join(changeDir, name)
		exists := false
		if info, err := os.Stat(path); err == nil {
			exists = name == "tests" && info.IsDir() || name != "tests" && !info.IsDir()
		}
		status := "missing"
		if exists {
			status = "present"
		}
		artifacts = append(artifacts, map[string]any{
			"name":   name,
			"path":   path,
			"status": status,
		})
	}
	taskTotal, taskDone := taskProgress(filepath.Join(changeDir, "task.md"))
	status := "incomplete"
	if allArtifactsPresent(artifacts) && taskTotal > 0 && taskDone == taskTotal {
		status = "ready"
	}
	return map[string]any{
		"change":     change,
		"status":     status,
		"artifacts":  artifacts,
		"acceptance": acceptanceSummary(filepath.Join(changeDir, "acceptance.json")),
		"tasks": map[string]int{
			"total": taskTotal,
			"done":  taskDone,
		},
	}
}

func changeArtifactNames() []string {
	// changeArtifactNames lists the active change artifacts that gate readiness.
	return []string{"brief.md", "proposal.md", "design.md", "spec.md", "task.md", "acceptance.json", "tests"}
}

func acceptanceSummary(path string) map[string]map[string]int {
	// acceptanceSummary exposes the hard contract size for status JSON consumers.
	summary := map[string]map[string]int{
		"coverage":          {"total": 0},
		"required_tests":    {"total": 0},
		"required_evidence": {"total": 0},
	}
	contract, err := acceptance.Read(path)
	if err != nil {
		return summary
	}
	summary["coverage"]["total"] = len(contract.Coverage)
	summary["required_tests"]["total"] = len(contract.RequiredTests)
	summary["required_evidence"]["total"] = len(contract.RequiredEvidence)
	return summary
}

func allArtifactsPresent(artifacts []map[string]any) bool {
	// allArtifactsPresent reports whether every fixed artifact exists.
	for _, artifact := range artifacts {
		if artifact["status"] != "present" {
			return false
		}
	}
	return true
}

func taskProgress(path string) (int, int) {
	// taskProgress counts markdown checkbox tasks in task.md.
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0
	}
	total, done := 0, 0
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [ ]") || strings.HasPrefix(trimmed, "- [x]") || strings.HasPrefix(trimmed, "- [X]") {
			total++
		}
		if strings.HasPrefix(trimmed, "- [x]") || strings.HasPrefix(trimmed, "- [X]") {
			done++
		}
	}
	return total, done
}
