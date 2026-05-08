// Package main provides the standalone oz CLI for the fixed Chinese SDD workflow.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/xbugs221/oz/skills"
)

var version = "dev"

type cli struct {
	out io.Writer
	err io.Writer
	now func() time.Time
}

type validationResult struct {
	Valid     bool              `json:"valid"`
	Change    string            `json:"change"`
	Errors    []string          `json:"errors"`
	Warnings  []string          `json:"warnings"`
	Artifacts map[string]string `json:"artifacts"`
}

func main() {
	// main maps CLI failures to process exit codes for shell and agent use.
	code := (&cli{out: os.Stdout, err: os.Stderr, now: time.Now}).run(os.Args[1:])
	os.Exit(code)
}

func (c *cli) run(args []string) int {
	// run dispatches the small non-interactive command surface.
	if c.now == nil {
		c.now = time.Now
	}
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		c.printHelp()
		return 0
	}
	if args[0] == "--version" || args[0] == "-v" {
		fmt.Fprintln(c.out, version)
		return 0
	}
	var err error
	switch args[0] {
	case "init":
		err = c.initCmd(args[1:])
	case "plan":
		err = c.stageCmd("plan")
	case "create":
		err = c.createCmd(args[1:])
	case "list":
		err = c.listCmd(args[1:])
	case "status":
		err = c.statusCmd(args[1:])
	case "exec":
		err = c.stageCmd("exec")
	case "validate":
		err = c.validateCmd(args[1:])
	case "archive":
		err = c.archiveCmd(args[1:])
	default:
		err = fmt.Errorf("unknown command: %s", args[0])
	}
	if err != nil {
		fmt.Fprintln(c.err, err)
		return 1
	}
	return 0
}

func (c *cli) printHelp() {
	// printHelp describes oz without mentioning removed Node or TypeScript workflows.
	fmt.Fprintln(c.out, "oz "+version)
	fmt.Fprintln(c.out, "")
	fmt.Fprintln(c.out, "Commands:")
	fmt.Fprintln(c.out, "  init [--global]")
	fmt.Fprintln(c.out, "  plan")
	fmt.Fprintln(c.out, "  create <change-name>")
	fmt.Fprintln(c.out, "  list [--json]")
	fmt.Fprintln(c.out, "  status <change> [--json]")
	fmt.Fprintln(c.out, "  exec")
	fmt.Fprintln(c.out, "  validate <change> [--json]")
	fmt.Fprintln(c.out, "  archive <change> --yes")
}

func (c *cli) initCmd(args []string) error {
	// initCmd installs the built-in agent skills into a project or user skill directory.
	global := hasArg(args, "--global")
	if len(args) > 1 || (len(args) == 1 && !global) {
		return errors.New("usage: oz init [--global]")
	}
	targetRoot := filepath.Join(".agents", "skills")
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		targetRoot = filepath.Join(home, ".agents", "skills")
	}
	builtIn, err := skills.BuiltIn()
	if err != nil {
		return err
	}
	for _, skill := range builtIn {
		target := filepath.Join(targetRoot, skill.Name, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(skill.Content), 0o644); err != nil {
			return err
		}
	}
	fmt.Fprintf(c.out, "Installed %d skills to %s\n", len(builtIn), targetRoot)
	return nil
}

func (c *cli) stageCmd(stage string) error {
	// stageCmd points agents at the repository skill templates for a workflow phase.
	fmt.Fprintf(c.out, "Run oz init, then use .agents/skills/oz-%s/SKILL.md for the %s stage.\n", stage, stage)
	return nil
}

func (c *cli) createCmd(args []string) error {
	// createCmd creates the numbered fixed artifact set for a new change.
	if len(args) != 1 {
		return errors.New("usage: oz create <change-name>")
	}
	name := args[0]
	if err := validateChangeName(name); err != nil {
		return err
	}
	root, err := stateRoot()
	if err != nil {
		return err
	}
	next, err := nextChangeNumber(root)
	if err != nil {
		return err
	}
	change := fmt.Sprintf("%d-%s", next, name)
	changeDir := filepath.Join(root, "changes", change)
	if err := os.MkdirAll(filepath.Join(changeDir, "tests"), 0o755); err != nil {
		return err
	}
	files := map[string]string{
		"proposal.md": "## 背景\n\n## 变更内容\n\n## 能力范围\n\n## 影响范围\n",
		"design.md":   "## 背景\n\n## 目标 / 非目标\n\n## 决策\n\n## 风险 / 取舍\n",
		"spec.md":     "## 新增需求\n\n### 需求：能力名称\n\n系统必须描述可验收行为。\n\n#### 场景：主要场景\n\n- **当** 触发条件发生\n- **则** 系统表现符合预期\n",
		"task.md":     "## 1. 实现\n\n- [ ] 1.1 完成实现并运行真实测试\n",
	}
	for file, body := range files {
		target := filepath.Join(changeDir, file)
		if err := writeNewFile(target, body); err != nil {
			return err
		}
	}
	fmt.Fprintf(c.out, "Created %s\n", changeDir)
	return nil
}

func (c *cli) listCmd(args []string) error {
	// listCmd reports active changes under docs/changes.
	jsonOut := hasArg(args, "--json")
	if len(args) > 1 || (len(args) == 1 && !jsonOut) {
		return errors.New("usage: oz list [--json]")
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

func (c *cli) statusCmd(args []string) error {
	// statusCmd reports fixed artifact presence and task progress for one active change.
	jsonOut := hasArg(args, "--json")
	change := firstPositional(args)
	if change == "" {
		return errors.New("usage: oz status <change> [--json]")
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
	fmt.Fprintf(c.out, "%s: %s\n", change, payload["status"])
	for _, artifact := range payload["artifacts"].([]map[string]any) {
		fmt.Fprintf(c.out, "- %s: %s\n", artifact["name"], artifact["status"])
	}
	return nil
}

func (c *cli) validateCmd(args []string) error {
	// validateCmd validates a fixed-format oz change and optionally emits stable JSON.
	jsonOut := hasArg(args, "--json")
	change := firstPositional(args)
	if change == "" {
		return errors.New("usage: oz validate <change> [--json]")
	}
	root, err := stateRoot()
	if err != nil {
		return err
	}
	result := validateChange(root, change)
	if jsonOut {
		_ = writeJSON(c.out, result)
	} else if result.Valid {
		fmt.Fprintf(c.out, "%s is valid\n", change)
	} else {
		for _, e := range result.Errors {
			fmt.Fprintln(c.err, e)
		}
	}
	if !result.Valid {
		return errors.New("validation failed")
	}
	return nil
}

func (c *cli) archiveCmd(args []string) error {
	// archiveCmd performs deterministic file moves; agents merge archived specs afterward.
	if len(args) == 0 {
		return errors.New("usage: oz archive <change> --yes")
	}
	change := args[0]
	if !hasArg(args[1:], "--yes") {
		return errors.New("archive requires --yes")
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
	projectRoot := filepath.Dir(root)
	changeDir := filepath.Join(root, "changes", change)
	testsDir := filepath.Join(changeDir, "tests")
	moves, err := plannedTestMoves(projectRoot, testsDir, date, change)
	if err != nil {
		return err
	}
	if len(moves) == 0 {
		return errors.New("archive requires at least one test file")
	}
	archiveDir := filepath.Join(root, "changes", "archive", date+"-"+change)
	if _, err := os.Stat(archiveDir); err == nil {
		return fmt.Errorf("archive target already exists: %s", archiveDir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, move := range moves {
		if _, err := os.Stat(move.dst); err == nil {
			return fmt.Errorf("test target already exists: %s", move.dst)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, "tests"), 0o755); err != nil {
		return err
	}
	for _, move := range moves {
		if err := os.Rename(move.src, move.dst); err != nil {
			return err
		}
	}
	if err := os.Remove(testsDir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(archiveDir), 0o755); err != nil {
		return err
	}
	if err := os.Rename(changeDir, archiveDir); err != nil {
		return err
	}
	fmt.Fprintf(c.out, "Archived to %s\n", archiveDir)
	return nil
}

type testMove struct {
	src string
	dst string
}

func plannedTestMoves(projectRoot, testsDir, date, change string) ([]testMove, error) {
	// plannedTestMoves builds all test moves before archive mutates the filesystem.
	entries, err := os.ReadDir(testsDir)
	if err != nil {
		return nil, err
	}
	moves := []testMove{}
	for _, entry := range entries {
		if entry.IsDir() {
			return nil, fmt.Errorf("tests directory only supports files: %s", entry.Name())
		}
		if !looksLikeTestCode(entry.Name()) {
			return nil, fmt.Errorf("tests directory contains non-test file: %s", entry.Name())
		}
		moves = append(moves, testMove{
			src: filepath.Join(testsDir, entry.Name()),
			dst: filepath.Join(projectRoot, "tests", date+"-"+change+"-"+entry.Name()),
		})
	}
	return moves, nil
}

func statusPayload(root, change string) map[string]any {
	// statusPayload summarizes fixed oz artifacts without dynamic workflow configuration.
	changeDir := filepath.Join(root, "changes", change)
	artifacts := []map[string]any{}
	for _, name := range []string{"proposal.md", "design.md", "spec.md", "task.md", "tests"} {
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
		"change":    change,
		"status":    status,
		"artifacts": artifacts,
		"tasks": map[string]int{
			"total": taskTotal,
			"done":  taskDone,
		},
	}
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

func validateChange(root, change string) validationResult {
	// validateChange checks naming, required artifacts, spec semantics, and test directory purpose.
	result := validationResult{
		Valid:     true,
		Change:    change,
		Errors:    []string{},
		Warnings:  []string{},
		Artifacts: map[string]string{},
	}
	if err := validateNumberedChange(change); err != nil {
		result.Errors = append(result.Errors, err.Error())
	}
	changeDir := filepath.Join(root, "changes", change)
	required := []string{"proposal.md", "design.md", "spec.md", "task.md"}
	for _, name := range required {
		path := filepath.Join(changeDir, name)
		result.Artifacts[name] = path
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			result.Errors = append(result.Errors, "missing "+name)
		}
	}
	testsDir := filepath.Join(changeDir, "tests")
	result.Artifacts["tests"] = testsDir
	if info, err := os.Stat(testsDir); err != nil || !info.IsDir() {
		result.Errors = append(result.Errors, "missing tests")
	} else if entries, err := os.ReadDir(testsDir); err != nil {
		result.Errors = append(result.Errors, "cannot read tests: "+err.Error())
	} else {
		for _, entry := range entries {
			if entry.IsDir() || !looksLikeTestCode(entry.Name()) {
				result.Errors = append(result.Errors, "tests contains non-test code: "+entry.Name())
			}
		}
	}
	if data, err := os.ReadFile(filepath.Join(changeDir, "spec.md")); err == nil {
		result.Errors = append(result.Errors, validateSpecText(string(data))...)
	}
	if data, err := os.ReadFile(filepath.Join(changeDir, "task.md")); err == nil && !strings.Contains(string(data), "- [") {
		result.Errors = append(result.Errors, "task.md must contain task items")
	}
	result.Errors = unique(result.Errors)
	result.Valid = len(result.Errors) == 0
	return result
}

func validateSpecText(text string) []string {
	// validateSpecText recognizes the minimum Chinese requirement, normative word, and scenario form.
	lines := strings.Split(text, "\n")
	hasReq, hasNorm, hasScenario := false, false, false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "### 需求：") {
			hasReq = true
		}
		if strings.Contains(trimmed, "必须") || strings.Contains(trimmed, "应当") || strings.Contains(trimmed, "不得") {
			hasNorm = true
		}
		if strings.HasPrefix(trimmed, "#### 场景：") {
			hasScenario = true
		}
	}
	errs := []string{}
	if !hasReq {
		errs = append(errs, "spec.md missing requirement")
	}
	if !hasNorm {
		errs = append(errs, "spec.md missing normative word")
	}
	if !hasScenario {
		errs = append(errs, "spec.md missing scenario")
	}
	return errs
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
			return errors.New("task.md contains unfinished tasks")
		}
	}
	return nil
}

func nextChangeNumber(root string) (int, error) {
	// nextChangeNumber scans active and archived changes and returns max prefix plus one.
	max := 0
	for _, dir := range []string{filepath.Join(root, "changes"), filepath.Join(root, "changes", "archive")} {
		entries, err := os.ReadDir(dir)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return 0, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if dir == filepath.Join(root, "changes", "archive") {
				parts := strings.SplitN(name, "-", 4)
				if len(parts) == 4 {
					name = parts[3]
				}
			}
			n := leadingNumber(name)
			if n > max {
				max = n
			}
		}
	}
	return max + 1, nil
}

func stateRoot() (string, error) {
	// stateRoot resolves the fixed docs state directory inside the current project.
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, "docs"), nil
}

func validateChangeName(name string) error {
	// validateChangeName accepts Chinese descriptions mixed with ASCII words, digits, and hyphens.
	if name == "" {
		return errors.New("change-name must not be empty")
	}
	hasChinese := false
	for i, r := range name {
		if isChinese(r) {
			hasChinese = true
			continue
		}
		valid := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || unicode.IsDigit(r) || r == '-'
		if !valid || (i == 0 && r == '-') {
			return errors.New("change-name may only contain Chinese characters, ASCII letters, digits, and hyphens")
		}
	}
	if !hasChinese {
		return errors.New("change-name 至少包含一个中文汉字")
	}
	return nil
}

func validateNumberedChange(change string) error {
	// validateNumberedChange verifies the archived and active change directory naming rule.
	re := regexp.MustCompile(`^[1-9][0-9]*-(.+)$`)
	matches := re.FindStringSubmatch(change)
	if matches == nil {
		return errors.New("change directory must be <number>-<change-name>")
	}
	return validateChangeName(matches[1])
}

func leadingNumber(name string) int {
	// leadingNumber extracts the numeric prefix used for local change ordering.
	parts := strings.SplitN(name, "-", 2)
	if len(parts) < 2 {
		return 0
	}
	n, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}
	return n
}

func looksLikeTestCode(name string) bool {
	// looksLikeTestCode keeps tests/ for executable project tests instead of notes.
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".txt") {
		return false
	}
	return strings.Contains(lower, "test") || strings.HasSuffix(lower, "_test.go") || strings.HasSuffix(lower, ".spec.ts")
}

func isChinese(r rune) bool {
	// isChinese checks the common CJK unified ideograph ranges used by Chinese filenames.
	return (r >= '\u4e00' && r <= '\u9fff') || (r >= '\u3400' && r <= '\u4dbf')
}

func writeNewFile(path, body string) error {
	// writeNewFile avoids silently overwriting user-authored proposal artifacts.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(body)
	return err
}

func hasArg(args []string, want string) bool {
	// hasArg reports whether a flag-like argument is present.
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func firstPositional(args []string) string {
	// firstPositional skips flags and returns the command target.
	for _, arg := range args {
		if !strings.HasPrefix(arg, "--") {
			return arg
		}
	}
	return ""
}

func unique(values []string) []string {
	// unique returns deterministic diagnostics with duplicates removed.
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func writeJSON(out io.Writer, payload any) error {
	// writeJSON emits stable indented JSON for scripts and agents.
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(data))
	return err
}
