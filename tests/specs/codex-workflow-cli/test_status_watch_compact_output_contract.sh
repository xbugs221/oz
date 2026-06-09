#!/usr/bin/env bash
# 文件功能目的：验证 wo status/watch 使用统一极简固定列视图，并且 batch 只是多个 workflow 视图的组合。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
result_dir="$repo_root/test-results/7-status-watch-compact-output"
test_file="$repo_root/internal/app/status_watch_compact_output_contract_test.go"

mkdir -p "$result_dir"
log="$result_dir/status-watch-compact-output.log"
: >"$log"

cleanup() {
  rm -f "$test_file"
}
trap cleanup EXIT

note() {
  # note 记录测试关键步骤，方便执行阶段复查合同失败点。
  printf '%s\n' "$*" | tee -a "$log"
}

cd "$repo_root"

note "写入 internal/app 包级契约测试，直接覆盖 wo status/watch 渲染路径"
cat >"$test_file" <<'GO'
package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStatusWatchCompactOutputContract 验证 status/watch 使用同一套极简固定列视图。
func TestStatusWatchCompactOutputContract(t *testing.T) {
	repo, state := compactStatusFixture(t)

	var status bytes.Buffer
	inRepo(t, repo, func() {
		if err := Run([]string{"status", "-w1"}, strings.NewReader(""), &status, &status); err != nil {
			t.Fatal(err)
		}
	})
	gotStatus := strings.TrimSpace(status.String())
	saveCompactResult(t, "status-w1.txt", gotStatus)

	wantStatusLines := []string{
		"→ w1",
		"规划阶段 planner-session ✓ 2.00",
		"执行阶段 writer-session → 6.50",
		"  代码侦察 subagent-session-1 ✓ 1.10",
		"  外部资料 subagent-session-2 ✓ 0.80",
		"审核阶段 reviewer-session - -",
		"测试阶段 - - -",
		"归档阶段 - - -",
	}
	for _, want := range wantStatusLines {
		if !hasExactLine(gotStatus, want) {
			t.Fatalf("status output missing exact line %q:\n%s", want, gotStatus)
		}
	}
	if !strings.HasPrefix(gotStatus, "→ w1\n") {
		t.Fatalf("status first line must be compact workflow header:\n%s", gotStatus)
	}
	for _, banned := range []string{"工作流", "批量任务", "引擎", "并行", "耗时", "implementation_context", "代码库侦察员", "外部资料研究员"} {
		if strings.Contains(gotStatus, banned) {
			t.Fatalf("status output should not contain %q:\n%s", banned, gotStatus)
		}
	}

	gotWatch := strings.Join(watchStatusLines(repo, "run", StatusRef{Alias: "w1", ID: state.RunID}, "|"), "\n")
	saveCompactResult(t, "watch-w1.txt", gotWatch)
	for _, want := range []string{
		"| w1",
		"规划阶段 planner-session ✓ 2.00",
		"执行阶段 writer-session → 6.50",
		"  代码侦察 subagent-session-1 ✓ 1.10",
		"  外部资料 subagent-session-2 ✓ 0.80",
	} {
		if !hasExactLine(gotWatch, want) {
			t.Fatalf("watch output missing exact line %q:\n%s", want, gotWatch)
		}
	}
	if !strings.HasPrefix(gotWatch, "| w1\n") {
		t.Fatalf("watch first line must use spinner header:\n%s", gotWatch)
	}

	batchID := "20260609T070001.000000000Z"
	batch := BatchState{
		BatchID:      batchID,
		Status:       batchStatusRunning,
		Changes:      []string{state.ChangeName, "8-待执行"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{state.ChangeName: state.RunID},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}
	gotBatch := strings.Join(watchStatusLines(repo, "batch", StatusRef{Alias: "b1", ID: batchID}, "|"), "\n")
	saveCompactResult(t, "watch-b1.txt", gotBatch)
	for _, want := range []string{
		"| b1 1/2",
		"- 7-统一输出",
		"  → w1",
		"  规划阶段 planner-session ✓ 2.00",
		"  执行阶段 writer-session → 6.50",
		"    代码侦察 subagent-session-1 ✓ 1.10",
		"- 8-待执行",
	} {
		if !hasExactLine(gotBatch, want) {
			t.Fatalf("batch watch output missing exact line %q:\n%s", want, gotBatch)
		}
	}
	if strings.Contains(gotBatch, "批量任务") || strings.Contains(gotBatch, "工作流") || strings.Contains(gotBatch, "并行") {
		t.Fatalf("batch output should only wrap compact workflow views:\n%s", gotBatch)
	}
}

// compactStatusFixture 创建真实仓库、run state、DAG node 和 subagent artifact，模拟正在执行阶段的业务状态。
func compactStatusFixture(t *testing.T) (string, State) {
	t.Helper()
	repo := gitRepo(t)
	changeName := "7-统一输出"
	changeDir := filepath.Join(repo, "docs", "changes", changeName)
	if err := os.MkdirAll(filepath.Join(changeDir, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"proposal.md", "design.md", "spec.md", "task.md", "acceptance.json"} {
		if err := os.WriteFile(filepath.Join(changeDir, name), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	workflow := DefaultWorkflowConfig()
	workflow.MaxReviewIterations = 1
	workflow.Parallel.Enabled = true
	workflow.Parallel.Groups["implementation_context"] = ParallelGroupConfig{
		Mode: "advisory",
		Members: []ParallelMemberConfig{
			{Name: "代码库侦察员", Purpose: "搜索现有模块", Tool: "pi", Subagent: "explore"},
			{Name: "外部资料研究员", Purpose: "查询外部资料", Tool: "pi", Subagent: "librarian"},
		},
	}

	runID := "20260609T070000.000000000Z"
	runPath := runDir(repo, runID)
	codeArtifact := filepath.Join(runPath, "parallel-members", "implementation_context", "code-scout.json")
	docsArtifact := filepath.Join(runPath, "parallel-members", "implementation_context", "external-docs.json")
	groupArtifact := filepath.Join(runPath, "parallel-implementation-context.json")

	state := State{
		RunID:      runID,
		ChangeName: changeName,
		Sealed:     true,
		Status:     statusRunning,
		Stage:      "execution",
		Engine:     "go-dag",
		Sessions: map[string]string{
			sessionStateKey("codex", "planner"): "planner-session",
			sessionStateKey("codex", "executor"): "writer-session",
			sessionStateKey("codex", "reviewer"): "reviewer-session",
			sessionStateKey("pi", "subagent:implementation_context:代码库侦察员:0"): "subagent-session-1",
			sessionStateKey("pi", "subagent:implementation_context:外部资料研究员:0"): "subagent-session-2",
		},
		Stages: map[string]string{"planning": "completed"},
		StageTimings: map[string]StageTiming{
			"planning":  {StartedAt: "2026-06-09T00:00:00Z", FinishedAt: "2026-06-09T00:02:00Z"},
			"execution": {StartedAt: "2026-06-09T00:02:00Z", FinishedAt: "2026-06-09T00:08:30Z"},
		},
		DAGNodes: map[string]DAGNodeState{
			"before_execution_1": {Status: "success", Artifact: codeArtifact, StartedAt: "2026-06-09T00:03:00Z", FinishedAt: "2026-06-09T00:04:06Z"},
			"before_execution_2": {Status: "success", Artifact: docsArtifact, StartedAt: "2026-06-09T00:03:00Z", FinishedAt: "2026-06-09T00:03:48Z"},
		},
		Paths:    map[string]string{},
		Workflow: workflow,
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(codeArtifact, ParallelMemberResult{Name: "代码库侦察员", Purpose: "搜索现有模块", Status: "success", Summary: "ok"}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(docsArtifact, ParallelMemberResult{Name: "外部资料研究员", Purpose: "查询外部资料", Status: "success", Summary: "ok"}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(groupArtifact, ParallelArtifact{
		Group: "implementation_context",
		Mode:  "advisory",
		Members: []ParallelMemberResult{
			{Name: "代码库侦察员", Purpose: "搜索现有模块", Status: "success", Summary: "ok"},
			{Name: "外部资料研究员", Purpose: "查询外部资料", Status: "success", Summary: "ok"},
		},
		Summary: "implementation context completed",
	}); err != nil {
		t.Fatal(err)
	}
	return repo, state
}

// inRepo 在临时仓库目录中执行命令解析，覆盖 GitRoot(".") 的真实路径。
func inRepo(t *testing.T, repo string, fn func()) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatal(err)
		}
	}()
	fn()
}

// hasExactLine 判断输出中是否存在完全相等的一行，避免子串误判。
func hasExactLine(output, want string) bool {
	for _, line := range strings.Split(output, "\n") {
		if line == want {
			return true
		}
	}
	return false
}

// saveCompactResult 保存本测试的中间输出，作为 acceptance runtime log 的补充材料。
func saveCompactResult(t *testing.T, name, body string) {
	t.Helper()
	dir := filepath.Join("..", "test-results", "7-status-watch-compact-output")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, name), []byte(body+"\n"), 0o644)
}
GO

note "运行 compact human 输出契约测试"
go test ./internal/app -run TestStatusWatchCompactOutputContract -count=1 2>&1 | tee -a "$log"
