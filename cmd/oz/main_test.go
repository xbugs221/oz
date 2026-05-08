// Package main tests the user-visible oz CLI workflow with real filesystem projects.
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type cliResult struct {
	code   int
	stdout string
	stderr string
}

func runCLI(t *testing.T, cwd string, args ...string) cliResult {
	// runCLI executes the CLI in-process from a temporary project directory.
	t.Helper()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(oldwd); err != nil {
			t.Fatal(err)
		}
	}()
	var stdout, stderr bytes.Buffer
	code := (&cli{
		out: &stdout,
		err: &stderr,
		now: func() time.Time {
			return time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
		},
	}).run(args)
	return cliResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func newProject(t *testing.T) string {
	// newProject creates the minimum docs tree oz expects in user projects.
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs", "changes", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeValidChange(t *testing.T, project, change string) {
	// writeValidChange creates a complete change with one real Go test file.
	t.Helper()
	dir := filepath.Join(project, "docs", "changes", change)
	if err := os.MkdirAll(filepath.Join(dir, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"proposal.md": "## 背景\n需要可追溯变更。\n\n## 变更内容\n- 实现 oz。\n",
		"design.md":   "## 背景\n固定工作流。\n\n## 决策\nCLI 先归档，智能体再合并主规格。\n",
		"spec.md":     "## 新增需求\n\n### 需求：归档测试\n\n系统必须移动测试文件。\n\n#### 场景：归档包含测试\n\n- **当** 用户归档提案\n- **则** 测试移动到根目录\n",
		"task.md":     "## 1. 实现\n\n- [x] 1.1 完成实现\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	testBody := "package tests\n\nimport \"testing\"\n\nfunc TestArchivedBehavior(t *testing.T) {}\n"
	if err := os.WriteFile(filepath.Join(dir, "tests", "archive_test.go"), []byte(testBody), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestHelpMentionsOzWithoutNodeTooling(t *testing.T) {
	// TestHelpMentionsOzWithoutNodeTooling covers the no-Node help scenario.
	result := runCLI(t, newProject(t), "--help")
	if result.code != 0 {
		t.Fatalf("help failed: %s", result.stderr)
	}
	if !strings.Contains(result.stdout, "oz ") || !strings.Contains(result.stdout, "init [--global]") || !strings.Contains(result.stdout, "create <change-name>") {
		t.Fatalf("help does not describe oz commands:\n%s", result.stdout)
	}
	for _, removed := range []string{"Node.js", "pnpm", "npm", "TypeScript", "ox "} {
		if strings.Contains(result.stdout, removed) {
			t.Fatalf("help mentions removed tooling %q:\n%s", removed, result.stdout)
		}
	}
}

func TestInitInstallsBuiltInSkillsIntoProject(t *testing.T) {
	// TestInitInstallsBuiltInSkillsIntoProject covers local agent skill installation.
	project := newProject(t)
	result := runCLI(t, project, "init")
	if result.code != 0 {
		t.Fatalf("init failed: %s", result.stderr)
	}
	for _, name := range []string{"oz-plan", "oz-create", "oz-exec", "oz-archive"} {
		data, err := os.ReadFile(filepath.Join(project, ".agents", "skills", name, "SKILL.md"))
		if err != nil {
			t.Fatalf("missing installed skill %s: %v", name, err)
		}
		if !strings.Contains(string(data), "name: "+name) {
			t.Fatalf("installed skill %s has wrong content:\n%s", name, string(data))
		}
	}
}

func TestInitGlobalInstallsBuiltInSkillsIntoHome(t *testing.T) {
	// TestInitGlobalInstallsBuiltInSkillsIntoHome covers user-level agent skill installation.
	project := newProject(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	result := runCLI(t, project, "init", "--global")
	if result.code != 0 {
		t.Fatalf("global init failed: %s", result.stderr)
	}
	data, err := os.ReadFile(filepath.Join(home, ".agents", "skills", "oz-archive", "SKILL.md"))
	if err != nil {
		t.Fatalf("missing global archive skill: %v", err)
	}
	if !strings.Contains(string(data), "oz archive <change> --yes") {
		t.Fatalf("global archive skill content missing archive command:\n%s", string(data))
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "skills")); !os.IsNotExist(err) {
		t.Fatalf("global init should not install project skills: %v", err)
	}
}

func TestCreateUsesChineseNumberedFixedArtifacts(t *testing.T) {
	// TestCreateUsesChineseNumberedFixedArtifacts covers numbering, Chinese names, and fixed files.
	project := newProject(t)
	if err := os.MkdirAll(filepath.Join(project, "docs", "changes", "3-已有提案"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, "docs", "changes", "archive", "2026-05-01-8-历史提案"), 0o755); err != nil {
		t.Fatal(err)
	}
	result := runCLI(t, project, "create", "重写-oz-go-cli")
	if result.code != 0 {
		t.Fatalf("create failed: %s", result.stderr)
	}
	if !strings.Contains(result.stdout, "9-重写-oz-go-cli") {
		t.Fatalf("create did not use next number from active and archive dirs:\n%s", result.stdout)
	}
	changeDir := filepath.Join(project, "docs", "changes", "9-重写-oz-go-cli")
	for _, rel := range []string{"proposal.md", "design.md", "spec.md", "task.md", "tests"} {
		if _, err := os.Stat(filepath.Join(changeDir, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(changeDir, "tests"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("tests directory should not contain placeholder files")
	}
}

func TestCreateRejectsEnglishOnlyChangeName(t *testing.T) {
	// TestCreateRejectsEnglishOnlyChangeName keeps proposal names Chinese-first.
	result := runCLI(t, newProject(t), "create", "rewrite-go-cli")
	if result.code == 0 {
		t.Fatal("expected create to reject English-only change name")
	}
	if !strings.Contains(result.stderr, "change-name 至少包含一个中文汉字") {
		t.Fatalf("unexpected error: %s", result.stderr)
	}
}

func TestListAndStatusReportActiveChangeProgress(t *testing.T) {
	// TestListAndStatusReportActiveChangeProgress covers lightweight inspection commands.
	project := newProject(t)
	writeValidChange(t, project, "2-重写-oz-go-cli")
	if err := os.MkdirAll(filepath.Join(project, "docs", "changes", "archive", "2026-05-01-1-历史提案"), 0o755); err != nil {
		t.Fatal(err)
	}
	list := runCLI(t, project, "list", "--json")
	if list.code != 0 {
		t.Fatalf("list failed: %s", list.stderr)
	}
	var listPayload struct {
		Changes []struct {
			Name string `json:"name"`
		} `json:"changes"`
	}
	if err := json.Unmarshal([]byte(list.stdout), &listPayload); err != nil {
		t.Fatalf("invalid list JSON: %v\n%s", err, list.stdout)
	}
	if len(listPayload.Changes) != 1 || listPayload.Changes[0].Name != "2-重写-oz-go-cli" {
		t.Fatalf("list should include only active changes: %#v", listPayload)
	}
	status := runCLI(t, project, "status", "2-重写-oz-go-cli", "--json")
	if status.code != 0 {
		t.Fatalf("status failed: %s", status.stderr)
	}
	var statusPayload struct {
		Change    string `json:"change"`
		Status    string `json:"status"`
		Artifacts []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"artifacts"`
		Tasks struct {
			Total int `json:"total"`
			Done  int `json:"done"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(status.stdout), &statusPayload); err != nil {
		t.Fatalf("invalid status JSON: %v\n%s", err, status.stdout)
	}
	if statusPayload.Change != "2-重写-oz-go-cli" || statusPayload.Status != "ready" {
		t.Fatalf("unexpected status payload: %#v", statusPayload)
	}
	if statusPayload.Tasks.Total != 1 || statusPayload.Tasks.Done != 1 {
		t.Fatalf("unexpected task progress: %#v", statusPayload.Tasks)
	}
	seen := map[string]string{}
	for _, artifact := range statusPayload.Artifacts {
		seen[artifact.Name] = artifact.Status
	}
	for _, name := range []string{"proposal.md", "design.md", "spec.md", "task.md", "tests"} {
		if seen[name] != "present" {
			t.Fatalf("artifact %s not present in status: %#v", name, seen)
		}
	}
}

func TestValidateOutputsStableJSON(t *testing.T) {
	// TestValidateOutputsStableJSON verifies valid and invalid proposal diagnostics.
	project := newProject(t)
	writeValidChange(t, project, "2-重写-oz-go-cli")
	result := runCLI(t, project, "validate", "2-重写-oz-go-cli", "--json")
	if result.code != 0 {
		t.Fatalf("validate failed: %s", result.stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, result.stdout)
	}
	if payload["valid"] != true || payload["change"] != "2-重写-oz-go-cli" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	for _, key := range []string{"errors", "warnings", "artifacts"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("missing JSON field %s: %#v", key, payload)
		}
	}
	if err := os.Remove(filepath.Join(project, "docs", "changes", "2-重写-oz-go-cli", "proposal.md")); err != nil {
		t.Fatal(err)
	}
	result = runCLI(t, project, "validate", "2-重写-oz-go-cli", "--json")
	if result.code == 0 {
		t.Fatal("expected invalid proposal to fail")
	}
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("invalid failure JSON: %v\n%s", err, result.stdout)
	}
	if payload["valid"] != false {
		t.Fatalf("expected valid=false: %#v", payload)
	}
}

func TestValidateReportsUnreadableTestsDirectory(t *testing.T) {
	// TestValidateReportsUnreadableTestsDirectory keeps JSON validation reliable for scripts.
	if os.Getuid() == 0 {
		t.Skip("root can read directories even after removing permission bits")
	}
	project := newProject(t)
	writeValidChange(t, project, "2-重写-oz-go-cli")
	testsDir := filepath.Join(project, "docs", "changes", "2-重写-oz-go-cli", "tests")
	if err := os.Chmod(testsDir, 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chmod(testsDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}()
	result := runCLI(t, project, "validate", "2-重写-oz-go-cli", "--json")
	if result.code == 0 {
		t.Fatal("expected unreadable tests directory to fail validation")
	}
	var payload struct {
		Valid  bool     `json:"valid"`
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, result.stdout)
	}
	if payload.Valid {
		t.Fatalf("expected valid=false: %#v", payload)
	}
	if !strings.Contains(strings.Join(payload.Errors, "\n"), "cannot read tests") {
		t.Fatalf("missing tests read error: %#v", payload.Errors)
	}
}

func TestArchiveMovesTestsWithoutEditingMainSpec(t *testing.T) {
	// TestArchiveMovesTestsWithoutEditingMainSpec keeps semantic spec merging outside the CLI.
	project := newProject(t)
	writeValidChange(t, project, "2-登录能力")
	mainSpec := filepath.Join(project, "docs", "specs", "oz-go-cli.md")
	if err := os.MkdirAll(filepath.Dir(mainSpec), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainSpec, []byte("主规格保持不变\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := runCLI(t, project, "archive", "2-登录能力", "--yes")
	if result.code != 0 {
		t.Fatalf("archive failed: %s", result.stderr)
	}
	if !strings.Contains(result.stdout, filepath.Join("archive")) {
		t.Fatalf("archive output missing target: %s", result.stdout)
	}
	if _, err := os.Stat(filepath.Join(project, "docs", "changes", "2-登录能力")); !os.IsNotExist(err) {
		t.Fatalf("active change still exists or stat failed differently: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(project, "tests", "*-2-登录能力-archive_test.go"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("archived test not moved, matches=%v err=%v", matches, err)
	}
	data, err := os.ReadFile(mainSpec)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "主规格保持不变\n" {
		t.Fatalf("archive edited main spec: %q", string(data))
	}
}

func TestArchiveRequiresAtLeastOneTestFile(t *testing.T) {
	// TestArchiveRequiresAtLeastOneTestFile enforces real test provenance at archive time.
	project := newProject(t)
	writeValidChange(t, project, "2-登录能力")
	if err := os.Remove(filepath.Join(project, "docs", "changes", "2-登录能力", "tests", "archive_test.go")); err != nil {
		t.Fatal(err)
	}
	result := runCLI(t, project, "archive", "2-登录能力", "--yes")
	if result.code == 0 {
		t.Fatal("expected archive to reject an empty tests directory")
	}
	if !strings.Contains(result.stderr, "archive requires at least one test file") {
		t.Fatalf("unexpected empty-tests error: %s", result.stderr)
	}
	if _, err := os.Stat(filepath.Join(project, "docs", "changes", "2-登录能力")); err != nil {
		t.Fatalf("change should remain active after empty-tests failure: %v", err)
	}
}

func TestArchiveStopsOnTestFileConflict(t *testing.T) {
	// TestArchiveStopsOnTestFileConflict prevents overwriting long-lived root tests.
	project := newProject(t)
	writeValidChange(t, project, "2-登录能力")
	if err := os.MkdirAll(filepath.Join(project, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	conflict := filepath.Join(project, "tests", "2026-05-08-2-登录能力-archive_test.go")
	if err := os.WriteFile(conflict, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := runCLI(t, project, "archive", "2-登录能力", "--yes")
	if result.code == 0 {
		t.Fatal("expected archive conflict")
	}
	if !strings.Contains(result.stderr, "test target already exists") {
		t.Fatalf("unexpected conflict error: %s", result.stderr)
	}
	data, err := os.ReadFile(conflict)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "keep me" {
		t.Fatal("archive overwrote conflicting root test")
	}
	if _, err := os.Stat(filepath.Join(project, "docs", "changes", "2-登录能力")); err != nil {
		t.Fatalf("change should remain active after conflict: %v", err)
	}
}
