#!/usr/bin/env bash
# 文件功能目的：验证默认 oz-flow.yaml 改为根节点树状 stages.before 配置，并验证 parallel 顶层开关控制子代理图节点。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
result_dir="$repo_root/test-results/codex-workflow-cli/tree-config"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$result_dir"
log="$result_dir/tree-config.log"
generated_yaml="$result_dir/generated-oz-flow.yaml"
graph_json="$result_dir/graph.json"
: >"$log"

note() {
  # note 同时写入 runtime log，作为 acceptance evidence producer。
  printf '%s\n' "$*" | tee -a "$log"
}

fail() {
  # fail 用业务级原因终止测试，避免只看到 shell 行号。
  note "FAIL: $*"
  exit 1
}

assert_contains() {
  local path="$1"
  local needle="$2"
  grep -qF "$needle" "$path" || fail "$path 缺少必需内容：$needle"
}

assert_not_regex() {
  local path="$1"
  local pattern="$2"
  if grep -Eq "$pattern" "$path"; then
    fail "$path 不应匹配旧字段模式：$pattern"
  fi
}

wo_bin="$tmp/wo"
empty_home="$tmp/empty-home"
mkdir -p "$empty_home"
note "构建真实 oz flow binary"
go build -C "$repo_root" -o "$wo_bin" ./cmd/oz 2>&1 | tee -a "$log"

project="$tmp/project"
mkdir -p "$project"
git -C "$project" init -q
git -C "$project" config user.email "test@example.com"
git -C "$project" config user.name "Test User"
printf 'demo\n' >"$project/README.md"
git -C "$project" add README.md
git -C "$project" commit -qm init

note "运行 oz flow config，生成默认 oz-flow.yaml"
(
  cd "$project"
  HOME="$empty_home" "$wo_bin" config
) >"$result_dir/config.out" 2>"$result_dir/config.err"

cp "$project/oz-flow.yaml" "$generated_yaml"
note "已生成 evidence: $generated_yaml"

for pattern in \
  '^wo:$' \
  '^[[:space:]]+workflow:$' \
  '^[[:space:]]+engine:' \
  '^[[:space:]]+defaults:' \
  '^[[:space:]]+iterations:' \
  '^[[:space:]]+permissions:' \
  '^[[:space:]]+cli:' \
  '^[[:space:]]+tool:' \
  '^[[:space:]]+groups:' \
  '^[[:space:]]+mode:' \
  '^[[:space:]]+model:'
do
  assert_not_regex "$generated_yaml" "$pattern"
done

assert_contains "$generated_yaml" "parallel: true"
assert_contains "$generated_yaml" "max_review_iterations: 5"
assert_contains "$generated_yaml" "stages:"
assert_contains "$generated_yaml" "execution:"
assert_contains "$generated_yaml" "review:"
assert_contains "$generated_yaml" "qa:"
assert_contains "$generated_yaml" "before:"
assert_contains "$generated_yaml" "agent: codex"
assert_contains "$generated_yaml" "agent: pi"

for name in \
  "代码库侦察员" \
  "外部资料研究员" \
  "目标核对审核员" \
  "测试有效性审核员" \
  "安全风险审核员" \
  "上下文一致性审核员" \
  "CLI/API 测试员" \
  "浏览器路径测试员" \
  "回归场景测试员"
do
  assert_contains "$generated_yaml" "$name"
done

enabled_graph="$tmp/graph-enabled.json"
disabled_graph="$tmp/graph-disabled.json"

note "导出 parallel:true graph，验证阶段前置子代理存在"
(
  cd "$project"
  HOME="$empty_home" "$wo_bin" graph --change demo --format json
) >"$enabled_graph" 2>"$result_dir/graph-enabled.err"

assert_contains "$enabled_graph" "代码库侦察员"
assert_contains "$enabled_graph" "浏览器路径测试员"
assert_contains "$enabled_graph" "execution"
assert_contains "$enabled_graph" "archive"

note "改写 oz-flow.yaml 为 parallel:false，验证子代理被整体关闭"
cat >"$project/oz-flow.yaml" <<'YAML'
parallel: false
max_review_iterations: 0
stages:
  execution:
    agent: codex
    reasoning: low
    before:
      - name: 代码库侦察员
        purpose: 搜索相关源码、测试、配置和既有实现约定
        agent: pi
        subagent: explore
        required: false
  archive:
    agent: codex
    reasoning: low
validation:
  limit: 2
  commands: []
YAML

(
  cd "$project"
  HOME="$empty_home" "$wo_bin" graph --change demo --format json
) >"$disabled_graph" 2>"$result_dir/graph-disabled.err"

assert_contains "$disabled_graph" "execution"
assert_contains "$disabled_graph" "archive"
if grep -qF "代码库侦察员" "$disabled_graph"; then
  fail "parallel:false 时 graph 不应包含 stages.execution.before 子代理"
fi

python3 - "$enabled_graph" "$disabled_graph" "$graph_json" <<'PY'
import json
import pathlib
import sys

enabled = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
disabled = json.loads(pathlib.Path(sys.argv[2]).read_text(encoding="utf-8"))
pathlib.Path(sys.argv[3]).write_text(
    json.dumps({"parallel_true": enabled, "parallel_false": disabled}, ensure_ascii=False, indent=2),
    encoding="utf-8",
)
PY

note "已生成 evidence: $graph_json"
note "PASS: tree-config-contract"
