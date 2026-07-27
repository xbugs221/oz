#!/usr/bin/env bash
# 文件目的：验证默认配置、DAG 图和人类 status 表达当前内嵌主阶段工作流。
# Sources: 3-默认启用-纯内嵌工作流并行subagents, 42-拆除固定子代理编排
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

note "oz flow config 应生成不含固定外置子代理的默认工作流"
(cd "$project" && "$wo" flow config) >"$tmp/config.out"
cat "$tmp/config.out" >>"$log"
cat "$project/oz-flow.yaml" >>"$log"
grep -qF "repair:" "$project/oz-flow.yaml" || fail "oz-flow.yaml 必须包含 repair 主阶段配置"
if rg -n 'parallel:|subagents:|subagent_guard:|before:' "$project/oz-flow.yaml" >>"$log"; then
  fail "oz-flow.yaml 不得重新引入固定外置子代理配置"
fi

note "oz flow graph mermaid 应展示当前动态质量循环"
(cd "$project" && "$wo" flow graph --change demo --format mermaid) >"$tmp/graph.mmd"
cat "$tmp/graph.mmd" >>"$log"
for want in "执行" "全量自查" "独立测试" "定向修复" "环境阻塞" "停滞阻塞" "归档"; do
  grep -qF "$want" "$tmp/graph.mmd" || fail "Mermaid graph 缺少 $want"
done
for forbidden in "planning_context" "implementation_context" "fan-in" "subagent" "review_2" "fix_2" "最多5轮"; do
  if grep -qF "$forbidden" "$tmp/graph.mmd"; then
    fail "Mermaid graph 不应暴露旧内部标签 $forbidden"
  fi
done

note "用长期 Go 测试验证 status 使用当前紧凑阶段视图"
go test ./internal/app -run TestGoDAGHumanStatusContract -count=1 -v 2>&1 | tee -a "$log"

note "PASS"
