#!/usr/bin/env bash
# 文件功能目的：验证 wo status/watch human 输出直接从变更提案列表开始，并把 watch spinner 放在 running 阶段行。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
result_dir="$repo_root/test-results/9-status-watch-proposal-list"
test_file="$repo_root/internal/app/status_watch_proposal_list_contract_test.go"
log="$result_dir/status-watch-proposal-list.log"

mkdir -p "$result_dir"
: >"$log"

cleanup() {
  rm -f "$test_file"
}
trap cleanup EXIT

note() {
  # note 记录合同执行步骤，便于执行阶段判断失败是否来自目标行为缺失。
  printf '%s\n' "$*" | tee -a "$log"
}

cd "$repo_root"

note "写入 internal/app 包级合同测试，覆盖真实 status/watch human 渲染路径"
cat >"$test_file" <<'GO'
// Package app validates the user-visible proposal-list contract for wo status/watch.
package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStatusWatchProposalListContract verifies status/watch start from change proposals, not runtime headers.
func TestStatusWatchProposalListContract(t *testing.T) {
	repo, batchID, state := proposalListFixture(t)

	var status bytes.Buffer
	if err := printHumanStatus(&status, repo); err != nil {
		t.Fatal(err)
	}
	gotStatus := strings.TrimSpace(status.String())
	saveProposalListResult(t, "status-default.txt", gotStatus)

	wantFirst := "- " + state.ChangeName
	if firstBusinessLine(gotStatus) != wantFirst {
		t.Fatalf("default status first business line = %q, want %q\nfull output:\n%s", firstBusinessLine(gotStatus), wantFirst, gotStatus)
	}
	for _, banned := range []string{"→ b1 1/1", "| b1 1/1", "→ w1", "| w1", "正在查看"} {
		if strings.Contains(gotStatus, banned) {
			t.Fatalf("default status must not show header or pre-list hint %q:\n%s", banned, gotStatus)
		}
	}
	if !hasExactLine(gotStatus, "  执行阶段 writer-session → -") {
		t.Fatalf("status must keep static running marker on execution row:\n%s", gotStatus)
	}

	gotBatchWatch := strings.Join(watchStatusLines(repo, "batch", StatusRef{Alias: "b1", ID: batchID}, "|"), "\n")
	saveProposalListResult(t, "watch-batch.txt", gotBatchWatch)
	if firstBusinessLine(gotBatchWatch) != wantFirst {
		t.Fatalf("batch watch first business line = %q, want %q\nfull output:\n%s", firstBusinessLine(gotBatchWatch), wantFirst, gotBatchWatch)
	}
	for _, banned := range []string{"| b1 1/1", "  → w1", "  | w1"} {
		if strings.Contains(gotBatchWatch, banned) {
			t.Fatalf("batch watch must not show runtime header %q:\n%s", banned, gotBatchWatch)
		}
	}
	if !hasExactLine(gotBatchWatch, "  执行阶段 writer-session | -") {
		t.Fatalf("batch watch must put spinner on running execution row:\n%s", gotBatchWatch)
	}

	gotRunWatch := strings.Join(watchStatusLines(repo, "run", StatusRef{Alias: "w1", ID: state.RunID}, "/"), "\n")
	saveProposalListResult(t, "watch-run.txt", gotRunWatch)
	if firstBusinessLine(gotRunWatch) != wantFirst {
		t.Fatalf("single-run watch first business line = %q, want %q\nfull output:\n%s", firstBusinessLine(gotRunWatch), wantFirst, gotRunWatch)
	}
	if strings.Contains(gotRunWatch, "/ w1") || strings.Contains(gotRunWatch, "→ w1") {
		t.Fatalf("single-run watch must not show workflow header:\n%s", gotRunWatch)
	}
	if !hasExactLine(gotRunWatch, "  执行阶段 writer-session / -") {
		t.Fatalf("single-run watch must put spinner on running execution row:\n%s", gotRunWatch)
	}
}

// proposalListFixture creates a realistic batch and run state for human status rendering.
func proposalListFixture(t *testing.T) (string, string, State) {
	t.Helper()
	repo := gitRepo(t)
	changeName := "9-长中文提案名用于验证status-watch直接展示提案列表"
	changeDir := filepath.Join(repo, "docs", "changes", changeName)
	if err := os.MkdirAll(filepath.Join(changeDir, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"brief.md", "proposal.md", "design.md", "spec.md", "task.md", "acceptance.json"} {
		if err := os.WriteFile(filepath.Join(changeDir, name), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	runID := "20260610T010000.000000000Z"
	state := State{
		RunID:      runID,
		ChangeName: changeName,
		Sealed:     true,
		Status:     statusRunning,
		Stage:      "execution",
		Engine:     "go-dag",
		Sessions: map[string]string{
			sessionStateKey("codex", "planner"):  "planner-session",
			sessionStateKey("codex", "executor"): "writer-session",
		},
		Stages:   map[string]string{"planning": "completed"},
		Paths:    map[string]string{},
		Workflow: DefaultWorkflowConfig(),
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}

	batchID := "20260610T010001.000000000Z"
	batch := BatchState{
		BatchID:      batchID,
		Status:       batchStatusRunning,
		Changes:      []string{changeName},
		CurrentIndex: 0,
		RunIDs:       map[string]string{changeName: runID},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	return repo, batchID, state
}

// firstBusinessLine returns the first non-empty line after trimming update-only noise.
func firstBusinessLine(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "更新可用：" {
			continue
		}
		return line
	}
	return ""
}

// hasExactLine reports whether output contains one exact visible line.
func hasExactLine(output, want string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimRight(line, "\r") == want {
			return true
		}
	}
	return false
}

// saveProposalListResult writes rendered output into test-results for review evidence.
func saveProposalListResult(t *testing.T, name string, body string) {
	t.Helper()
	dir := filepath.Join("..", "test-results", "9-status-watch-proposal-list")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
GO

note "运行 Go 合同测试；当前实现预期失败于顶部 header 和 spinner 位置"
if ! go test ./internal/app -run TestStatusWatchProposalListContract -count=1 2>&1 | tee -a "$log"; then
  note "合同测试失败，若失败点是 header/spinner 行为缺失，则符合创建阶段预期"
  exit 1
fi

note "PASS"
