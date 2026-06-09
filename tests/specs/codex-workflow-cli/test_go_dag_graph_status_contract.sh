#!/usr/bin/env bash
# 文件目的：验证默认配置、DAG 图和人类 status 都能清晰表达 go-dag 与并行 subagents。
# Sources: 3-默认启用-纯go-dag并行subagents
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

log_dir="$repo_root/test-results/go-dag"
mkdir -p "$log_dir"
log="$log_dir/go-dag-graph-status-contract.log"
: >"$log"

# note 记录每个业务断言，方便失败后从 runtime log 复查。
note() {
  printf '%s\n' "$*" | tee -a "$log"
}

# fail 用统一错误出口，避免测试半成功。
fail() {
  note "FAIL: $*"
  exit 1
}

cd "$repo_root"
wo="$tmp/wo"
go build -o "$wo" ./cmd/wo 2>&1 | tee -a "$log"

project="$tmp/project"
mkdir -p "$project"
git -C "$project" init -q
git -C "$project" config user.email "test@example.com"
git -C "$project" config user.name "Test User"
printf 'initial\n' >"$project/README.md"
git -C "$project" add README.md
git -C "$project" commit -q -m initial

note "wo config 应生成默认 go-dag 与 parallel enabled 配置"
(cd "$project" && "$wo" config) >"$tmp/config.out"
cat "$tmp/config.out" >>"$log"
cat "$project/wo.yaml" >>"$log"
grep -qF "engine: go-dag" "$project/wo.yaml" || fail "wo.yaml 必须声明默认 engine: go-dag"
grep -qF "parallel:" "$project/wo.yaml" || fail "wo.yaml 必须包含 parallel 配置"
grep -qF "enabled: true" "$project/wo.yaml" || fail "parallel.enabled 必须默认 true"

note "wo graph mermaid 应展示紧凑中文状态图和默认并行成员"
(cd "$project" && "$wo" graph --change demo --format mermaid) >"$tmp/graph.mmd"
cat "$tmp/graph.mmd" >>"$log"
for want in \
  "需求分析员" \
  "代码库侦察员" \
  "外部资料研究员" \
  "规划上下文" \
  "执行上下文" \
  "审核" \
  "测试" \
  "修复" \
  "最多5轮" \
  "归档"
do
  grep -qF "$want" "$tmp/graph.mmd" || fail "Mermaid graph 缺少 $want"
done
for forbidden in \
  "planning_context" \
  "implementation_context" \
  "fan-in" \
  "subagent" \
  "review_2" \
  "qa_2" \
  "fix_2"
do
  if grep -qF "$forbidden" "$tmp/graph.mmd"; then
    fail "Mermaid graph 不应暴露旧内部标签 $forbidden"
  fi
done
if grep -qi "dagu" "$tmp/graph.mmd"; then
  fail "默认 Mermaid graph 不应要求 Dagu CLI"
fi

note "用 run-local state 验证 status 能展示 engine 和并行成员明细"
test_file="$repo_root/internal/app/go_dag_status_contract_test.go"
trap 'rm -rf "$tmp"; rm -f "$test_file"' EXIT
cat >"$test_file" <<'GO'
package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGoDAGHumanStatusContract verifies the user-facing status summary for the default Go DAG engine.
func TestGoDAGHumanStatusContract(t *testing.T) {
	repo := gitRepo(t)
	workflow := DefaultWorkflowConfig()
	workflow.Parallel.Enabled = true
	state := State{
		RunID:      "go-dag-status-run",
		ChangeName: "demo",
		Sealed:     true,
		Status:     statusRunning,
		Stage:      "review_1",
		Sessions:   map[string]string{"codex:executor": "exec-session"},
		Stages:     map[string]string{"execution": "completed"},
		Workflow:   workflow,
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	base := runDir(repo, state.RunID)
	mustWritePrompt(t, filepath.Join(base, "parallel-planning-context.json"), parallelPlanningArtifactForTest())
	mustWritePrompt(t, filepath.Join(base, "parallel-implementation-context.json"), parallelContextArtifactForTest())
	mustWritePrompt(t, filepath.Join(base, "parallel-review-1.json"), parallelReviewArtifactForTest())
	var stdout bytes.Buffer
	if err := printHumanStatus(&stdout, repo, "-w1"); err != nil {
		t.Fatal(err)
	}
	text := stdout.String()
	for _, want := range []string{
		"引擎 go-dag",
		"并行 planning_context 3/3 success",
		"需求分析员 success",
		"并行 implementation_context 2/2 success",
		"并行 review 5/5 success",
		"目标核对审核员 success",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("status output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "dagu") {
		t.Fatalf("go-dag status should not expose Dagu CLI details:\n%s", text)
	}
	_ = os.RemoveAll(repo)
}
GO

go test ./internal/app -run TestGoDAGHumanStatusContract -count=1 2>&1 | tee -a "$log"

note "PASS"
