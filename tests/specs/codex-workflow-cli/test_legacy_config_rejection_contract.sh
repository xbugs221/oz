#!/usr/bin/env bash
# 文件功能目的：验证旧 oz-flow.yaml 顶层、别名和冗余字段被硬拒绝，不再被静默兼容。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
result_dir="$repo_root/test-results/codex-workflow-cli/tree-config"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$result_dir"
log="$result_dir/legacy-config-rejection.log"
: >"$log"

note() {
  # note 生成 legacy-config-rejection-log 证据。
  printf '%s\n' "$*" | tee -a "$log"
}

fail() {
  # fail 输出业务语义，方便执行阶段定位是哪个旧字段仍被接受。
  note "FAIL: $*"
  exit 1
}

oz_bin="$tmp/wo"
empty_home="$tmp/empty-home"
mkdir -p "$empty_home"
note "构建真实 oz flow binary"
go build -C "$repo_root" -o "$oz_bin" ./cmd/oz 2>&1 | tee -a "$log"

init_project() {
  local project="$1"
  mkdir -p "$project"
  git -C "$project" init -q
  git -C "$project" config user.email "test@example.com"
  git -C "$project" config user.name "Test User"
  printf 'demo\n' >"$project/README.md"
  git -C "$project" add README.md
  git -C "$project" commit -qm init
}

run_rejection_case() {
  local name="$1"
  local field="$2"
  local project="$tmp/$name"
  init_project "$project"
  cat >"$project/oz-flow.yaml"

  note "验证旧字段被拒绝：$field ($name)"
  set +e
  (
    cd "$project"
    HOME="$empty_home" "$oz_bin" flow graph --change demo --format json
  ) >"$tmp/$name.out" 2>"$tmp/$name.err"
  local code=$?
  set -e

  cat "$tmp/$name.out" "$tmp/$name.err" >>"$log"
  if [[ "$code" -eq 0 ]]; then
    fail "旧字段 $field 仍被接受：$name"
  fi
  if ! grep -Eiq "$field" "$tmp/$name.out" "$tmp/$name.err"; then
    fail "旧字段 $field 的失败诊断没有包含字段名：$name"
  fi
}

run_global_rejection_case() {
  local name="$1"
  local field="$2"
  local project="$tmp/$name-project"
  local home="$tmp/$name-home"
  init_project "$project"
  mkdir -p "$home"
  cat >"$home/oz-flow.yaml"

  note "验证旧全局配置被拒绝：$field ($name)"
  set +e
  (
    cd "$project"
    HOME="$home" "$oz_bin" flow graph --change demo --format json
  ) >"$tmp/$name.out" 2>"$tmp/$name.err"
  local code=$?
  set -e

  cat "$tmp/$name.out" "$tmp/$name.err" >>"$log"
  if [[ "$code" -eq 0 ]]; then
    fail "旧全局字段 $field 仍被接受：$name"
  fi
  if ! grep -Eiq "$field" "$tmp/$name.out" "$tmp/$name.err"; then
    fail "旧全局字段 $field 的失败诊断没有包含字段名：$name"
  fi
}

run_rejection_case "old-oz-flow-root" "wo" <<'YAML'
wo:
  workflow:
    max_review_iterations: 0
    stages:
      execution:
        cli: codex
YAML

run_rejection_case "old-workflow-root" "workflow" <<'YAML'
workflow:
  max_review_iterations: 0
  stages:
    execution:
      agent: codex
YAML

run_rejection_case "old-engine" "engine" <<'YAML'
engine: 内嵌工作流
max_review_iterations: 0
stages:
  execution:
    agent: codex
YAML

run_rejection_case "old-defaults" "defaults" <<'YAML'
parallel: true
defaults:
  agent: codex
stages:
  execution:
    agent: codex
YAML

run_rejection_case "old-iterations" "iterations" <<'YAML'
parallel: true
max_review_iterations: 1
stages:
  execution:
    agent: codex
iterations:
  fix_1:
    model: old-model
YAML

run_rejection_case "old-cli" "cli" <<'YAML'
parallel: true
stages:
  execution:
    cli: codex
YAML

run_rejection_case "old-tool" "tool" <<'YAML'
parallel: true
stages:
  execution:
    tool: codex
YAML

run_rejection_case "old-before-cli" "cli" <<'YAML'
parallel: true
stages:
  execution:
    agent: codex
    before:
      - name: 代码库侦察员
        purpose: 搜索相关源码
        cli: pi
YAML

run_rejection_case "old-before-tool" "tool" <<'YAML'
parallel: true
stages:
  execution:
    agent: codex
    before:
      - name: 代码库侦察员
        purpose: 搜索相关源码
        tool: pi
YAML

run_rejection_case "old-before-stage" "stage" <<'YAML'
parallel: true
stages:
  execution:
    agent: codex
    before:
      - name: 代码库侦察员
        purpose: 搜索相关源码
        agent: pi
        stage: before_execution
YAML

run_rejection_case "old-permissions" "permissions" <<'YAML'
parallel: true
stages:
  execution:
    agent: codex
    permissions: danger-full-access
YAML

run_rejection_case "old-parallel-groups" "groups" <<'YAML'
parallel:
  enabled: true
  groups:
    implementation_context:
      mode: advisory
      members:
        - name: 代码库侦察员
          purpose: 搜索相关源码
          tool: pi
stages:
  execution:
    agent: codex
YAML

run_rejection_case "old-mode" "mode" <<'YAML'
parallel: true
stages:
  execution:
    agent: codex
    before:
      - name: 代码库侦察员
        purpose: 搜索相关源码
        agent: pi
        mode: advisory
YAML

run_rejection_case "old-validation-max" "max_attempts_per_stage" <<'YAML'
parallel: true
stages:
  execution:
    agent: codex
validation:
  max_attempts_per_stage: 3
YAML

run_global_rejection_case "old-global-oz-flow-root" "wo" <<'YAML'
wo:
  workflow:
    stages:
      execution:
        cli: codex
YAML

note "PASS: legacy-config-rejection-contract"
