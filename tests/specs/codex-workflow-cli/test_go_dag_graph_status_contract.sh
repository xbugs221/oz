#!/usr/bin/env bash
# 文件目的：验证默认配置、DAG 图和人类 status 都能清晰表达 内嵌工作流 与并行 subagents。
# Sources: 3-默认启用-纯内嵌工作流并行subagents
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

log_dir="$repo_root/test-results/workflow"
mkdir -p "$log_dir"
log="$log_dir/内嵌工作流-graph-status-contract.log"
: >"$log"

note() { printf '%s
' "$*" | tee -a "$log"; }
fail() { note "FAIL: $*"; exit 1; }

cd "$repo_root"
wo="$tmp/wo"
go build -o "$wo" ./cmd/oz 2>&1 | tee -a "$log"

project="$tmp/project"
mkdir -p "$project"
git -C "$project" init -q
git -C "$project" config user.email "test@example.com"
git -C "$project" config user.name "Test User"
printf 'initial
' >"$project/README.md"
git -C "$project" add README.md
git -C "$project" commit -q -m initial

note "oz flow config 应生成默认工作流 与 parallel enabled 配置"
(cd "$project" && "$wo" flow config) >"$tmp/config.out"
cat "$tmp/config.out" >>"$log"
cat "$project/oz-flow.yaml" >>"$log"
grep -qF "parallel:" "$project/oz-flow.yaml" || fail "oz-flow.yaml 必须包含 parallel 配置"
grep -qF "parallel: true" "$project/oz-flow.yaml" || fail "parallel 必须默认启用"

note "oz flow graph mermaid 应展示紧凑中文状态图和默认并行成员"
(cd "$project" && "$wo" flow graph --change demo --format mermaid) >"$tmp/graph.mmd"
cat "$tmp/graph.mmd" >>"$log"
for want in "代码库侦察员" "外部资料研究员" "执行上下文" "审核" "测试" "修复" "最多5轮" "归档"; do
  grep -qF "$want" "$tmp/graph.mmd" || fail "Mermaid graph 缺少 $want"
done
for forbidden in "planning_context" "implementation_context" "fan-in" "subagent" "review_2" "qa_2" "fix_2"; do
  if grep -qF "$forbidden" "$tmp/graph.mmd"; then
    fail "Mermaid graph 不应暴露旧内部标签 $forbidden"
  fi
done

note "用长期 Go 测试验证 status 使用当前紧凑阶段视图"
go test ./internal/app -run TestGoDAGHumanStatusContract -count=1 -v 2>&1 | tee -a "$log"

note "PASS"
