// Package main provides the standalone oz CLI for installing skills and inspecting oz changes.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/xbugs221/oz/skills"
)

var (
	version                = "dev"
	activeChangeNumberRe   = regexp.MustCompile(`^([1-9][0-9]*)-.+$`)
	archivedChangeNumberRe = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}-([1-9][0-9]*)-.+$`)
)

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
		fmt.Fprintln(c.out, resolvedVersion())
		return 0
	}
	var err error
	switch args[0] {
	case "list", "l":
		err = c.listCmd(args[1:])
	case "install", "i":
		err = c.installCmd(args[1:])
	case "create":
		err = c.createCmd(args[1:])
	case "status":
		err = c.statusCmd(args[1:])
	case "validate":
		err = c.validateCmd(args[1:])
	case "archive":
		err = c.archiveCmd(args[1:])
	default:
		err = fmt.Errorf("未知命令：%s", args[0])
	}
	if err != nil {
		fmt.Fprintln(c.err, err)
		return 1
	}
	return 0
}

func (c *cli) printHelp() {
	// printHelp separates user commands from automation interfaces.
	fmt.Fprintln(c.out, "oz "+resolvedVersion())
	fmt.Fprintln(c.out, "")
	fmt.Fprintln(c.out, "日常命令：")
	fmt.Fprintln(c.out, "  list | l [--json]")
	fmt.Fprintln(c.out, "  install | i [--global | -g]")
	fmt.Fprintln(c.out, "")
	fmt.Fprintln(c.out, "自动化接口：")
	fmt.Fprintln(c.out, "  create")
	fmt.Fprintln(c.out, "  status <change> [--json]")
	fmt.Fprintln(c.out, "  validate <change> [--json]")
	fmt.Fprintln(c.out, "  archive <change> --yes")
}

func resolvedVersion() string {
	// resolvedVersion reports the release tag when oz was installed or run from the source repository.
	if version != "" && version != "dev" {
		return version
	}
	if tag, err := sourceGitTag(); err == nil && tag != "" {
		return tag
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

func sourceGitTag() (string, error) {
	// sourceGitTag reads git describe only when the current repository is oz itself.
	root, err := sourceRoot()
	if err != nil {
		return "", err
	}
	describeCmd := exec.Command("git", "-C", root, "describe", "--tags", "--always", "--dirty")
	describeOut, err := describeCmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(describeOut)), nil
}

func sourceRoot() (string, error) {
	// sourceRoot locates the oz checkout from compiled source paths before consulting the current directory.
	if _, file, _, ok := runtime.Caller(0); ok {
		if root, err := ozModuleRoot(filepath.Dir(file)); err == nil {
			return root, nil
		}
	}
	rootCmd := exec.Command("git", "rev-parse", "--show-toplevel")
	rootOut, err := rootCmd.Output()
	if err != nil {
		return "", err
	}
	return ozModuleRoot(strings.TrimSpace(string(rootOut)))
}

func ozModuleRoot(start string) (string, error) {
	// ozModuleRoot walks upward until it finds the oz module root.
	for dir := start; ; dir = filepath.Dir(dir) {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && strings.Contains(string(data), "module github.com/xbugs221/oz\n") {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("oz module root not found")
		}
	}
}

func (c *cli) installCmd(args []string) error {
	// installCmd installs built-in agent skills into a project or user skill directory.
	if hasHelp(args) {
		c.printInstallHelp()
		return nil
	}
	global := hasArg(args, "--global") || hasArg(args, "-g")
	if len(args) > 1 || (len(args) == 1 && !global) {
		return errors.New("用法：oz install [--global | -g]")
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
	fmt.Fprintf(c.out, "已安装 %d 个技能到 %s\n", len(builtIn), targetRoot)
	return nil
}

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

func (c *cli) printInstallHelp() {
	// printInstallHelp documents local and global skill installation forms.
	fmt.Fprintln(c.out, "用法：")
	fmt.Fprintln(c.out, "  oz install [--global | -g]")
	fmt.Fprintln(c.out, "  oz i [--global | -g]")
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

func (c *cli) validateCmd(args []string) error {
	// validateCmd validates a fixed-format oz change and optionally emits stable JSON.
	if hasHelp(args) {
		fmt.Fprintln(c.out, "用法：oz validate <change> [--json]")
		return nil
	}
	jsonOut := hasArg(args, "--json")
	change := firstPositional(args)
	if change == "" {
		return errors.New("用法：oz validate <change> [--json]")
	}
	root, err := stateRoot()
	if err != nil {
		return err
	}
	result := validateChange(root, change)
	if jsonOut {
		_ = writeJSON(c.out, result)
	} else if result.Valid {
		fmt.Fprintf(c.out, "%s 校验通过\n", change)
	} else {
		for _, e := range result.Errors {
			fmt.Fprintln(c.err, e)
		}
	}
	if !result.Valid {
		return errors.New("校验失败")
	}
	return nil
}

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
	projectRoot := filepath.Dir(root)
	changeDir := filepath.Join(root, "changes", change)
	testsDir := filepath.Join(changeDir, "tests")
	moves, err := plannedTestMoves(projectRoot, testsDir, date, change)
	if err != nil {
		return err
	}
	if len(moves) == 0 {
		return errors.New("归档至少需要一个测试文件")
	}
	archiveDir := filepath.Join(root, "changes", "archive", date+"-"+change)
	if _, err := os.Stat(archiveDir); err == nil {
		return fmt.Errorf("归档目标已存在：%s", archiveDir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, move := range moves {
		if _, err := os.Stat(move.dst); err == nil {
			return fmt.Errorf("测试目标已存在：%s", move.dst)
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
	fmt.Fprintf(c.out, "已归档到 %s\n", archiveDir)
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
			return nil, fmt.Errorf("tests 目录只支持文件：%s", entry.Name())
		}
		if !looksLikeTestCode(entry.Name()) {
			return nil, fmt.Errorf("tests 目录包含非测试文件：%s", entry.Name())
		}
		moves = append(moves, testMove{
			src: filepath.Join(testsDir, entry.Name()),
			dst: filepath.Join(projectRoot, "tests", date+"-"+change+"-"+entry.Name()),
		})
	}
	return moves, nil
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
			result.Errors = append(result.Errors, "缺少 "+name)
		}
	}
	testsDir := filepath.Join(changeDir, "tests")
	result.Artifacts["tests"] = testsDir
	if info, err := os.Stat(testsDir); err != nil || !info.IsDir() {
		result.Errors = append(result.Errors, "缺少 tests")
	} else if entries, err := os.ReadDir(testsDir); err != nil {
		result.Errors = append(result.Errors, "无法读取 tests："+err.Error())
	} else {
		if len(entries) == 0 {
			result.Errors = append(result.Errors, "tests 必须包含至少一个测试文件")
		}
		for _, entry := range entries {
			if entry.IsDir() || !looksLikeTestCode(entry.Name()) {
				result.Errors = append(result.Errors, "tests 包含非测试代码："+entry.Name())
			}
		}
	}
	if data, err := os.ReadFile(filepath.Join(changeDir, "spec.md")); err == nil {
		result.Errors = append(result.Errors, validateSpecText(string(data))...)
	}
	if data, err := os.ReadFile(filepath.Join(changeDir, "task.md")); err == nil && !strings.Contains(string(data), "- [") {
		result.Errors = append(result.Errors, "task.md 必须包含任务项")
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
		errs = append(errs, "spec.md 缺少需求")
	}
	if !hasNorm {
		errs = append(errs, "spec.md 缺少规范词")
	}
	if !hasScenario {
		errs = append(errs, "spec.md 缺少场景")
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
			return errors.New("task.md 包含未完成任务")
		}
	}
	return nil
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
		return errors.New("change-name 不能为空")
	}
	hasChinese := false
	for i, r := range name {
		if isChinese(r) {
			hasChinese = true
			continue
		}
		valid := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || unicode.IsDigit(r) || r == '-'
		if !valid || (i == 0 && r == '-') {
			return errors.New("change-name 只能包含中文汉字、ASCII 字母、数字和连字符")
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
		return errors.New("变更目录必须符合 <number>-<change-name>")
	}
	return validateChangeName(matches[1])
}

func displayStatus(status string) string {
	// displayStatus localizes human-readable status values while JSON keeps stable machine strings.
	switch status {
	case "present":
		return "已存在"
	case "missing":
		return "缺失"
	case "ready":
		return "可归档"
	case "incomplete":
		return "未完成"
	default:
		return status
	}
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

func hasArg(args []string, want string) bool {
	// hasArg reports whether a flag-like argument is present.
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func hasHelp(args []string) bool {
	// hasHelp reports whether a command-specific help flag was requested.
	return hasArg(args, "--help") || hasArg(args, "-h")
}

func firstPositional(args []string) string {
	// firstPositional skips flags and returns the command target.
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
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
