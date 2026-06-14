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
go build -o "$wo" ./cmd/oz 2>&1 | tee -a "$log"

project="$tmp/project"
mkdir -p "$project"
git -C "$project" init -q
git -C "$project" config user.email "test@example.com"
git -C "$project" config user.name "Test User"
printf 'initial\n' >"$project/README.md"
git -C "$project" add README.md
git -C "$project" commit -q -m initial

note "oz flow config 应生成默认 go-dag 与 parallel enabled 配置"
(cd "$project" && "$wo" config) >"$tmp/config.out"
cat "$tmp/config.out" >>"$log"
cat "$project/oz-flow.yaml" >>"$log"
grep -qF "engine: go-dag" "$project/oz-flow.yaml" || fail "oz-flow.yaml 必须声明默认 engine: go-dag"
grep -qF "parallel:" "$project/oz-flow.yaml" || fail "oz-flow.yaml 必须包含 parallel 配置"
grep -qF "enabled: true" "$project/oz-flow.yaml" || fail "parallel.enabled 必须默认 true"

note "oz flow graph mermaid 应展示紧凑中文状态图和默认并行成员"
(cd "$project" && "$wo" graph --change demo --format mermaid) >"$tmp/graph.mmd"
cat "$tmp/graph.mmd" >>"$log"
for want in \
  "代码库侦察员" \
  "外部资料研究员" \
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
note "用 run-local state 验证 status 使用当前紧凑阶段视图"
test_file="$repo_root/tests/app/go_dag_status_contract_test.gotest"
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
		"- demo → -",
		"  执行 exec-session ✓ -",
		"  审核 -            → -",
		"  修正 -            -  -",
		"  测试 -            -  -",
		"  归档 -            -  -",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("status output missing %q:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{"引擎 go-dag", "- 并行", "planning_context", "implementation_context", "parallel-review"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("status output leaked internal status text %q:\n%s", forbidden, text)
		}
	}
	_ = os.RemoveAll(repo)
}
GO

OZ_MIGRATED_APP_RUN=TestGoDAGHumanStatusContract \
  go test ./tests/app -run TestMigratedAppTestsRunWithGoToolchain -count=1 2>&1 | tee -a "$log"

note "PASS"
