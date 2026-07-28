#!/usr/bin/env bash
# 文件功能目的：验证 PATH 中真实安装的 oz 默认生成无固定轮次上限的动态质量循环配置。

set -euo pipefail

REPO_ROOT="$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel)"
EVIDENCE_ROOT="$REPO_ROOT/test-results/46-installed-quality-loop"
RUNTIME_LOG="$EVIDENCE_ROOT/runtime.log"
TEMP_ROOT="$(mktemp -d)"

# cleanup 删除临时 Git 仓库，避免合同测试污染工作区。
cleanup() {
  rm -rf "$TEMP_ROOT"
}
trap cleanup EXIT

# record_runtime 记录真实安装版和配置检查结果，供独立 QA 复核。
record_runtime() {
  local oz_path
  oz_path="$(command -v oz)"
  {
    printf 'oz_path=%s\n' "$oz_path"
    printf 'oz_version='
    (cd "$TEMP_ROOT" && oz --version)
  } >"$RUNTIME_LOG"
}

# generate_default_config 在隔离 Git 仓库调用真实安装版生成默认配置。
generate_default_config() {
  git -C "$TEMP_ROOT" init -q
  (cd "$TEMP_ROOT" && oz flow config)
  test -s "$TEMP_ROOT/oz-flow.yaml"
}

# prompt_block 从生成配置中提取指定提示词块，避免跨角色关键词造成假阳性。
prompt_block() {
  local prompt_name="$1"
  awk -v prompt_name="$prompt_name" '
    $0 == "  " prompt_name ": |" { inside = 1; next }
    inside && /^  [a-z_]+: \|$/ { exit }
    inside { print }
  ' "$TEMP_ROOT/oz-flow.yaml"
}

# verify_dynamic_contract 核对默认配置只表达动态质量结果，不保留固定轮次终止条件。
verify_dynamic_contract() {
  local repair_prompt
  local qa_prompt

  if rg -q '^max_repair_iterations:' "$TEMP_ROOT/oz-flow.yaml"; then
    printf '默认配置仍包含固定修复轮次\n' >&2
    return 1
  fi
  repair_prompt="$(prompt_block repair)"
  qa_prompt="$(prompt_block qa)"
  rg -q '`pre_qa_audit`（`audit_N`）' <<<"$repair_prompt"
  rg -q '`qa_targeted_repair`（`targeted_repair_N`）' <<<"$repair_prompt"
  rg -q 'repairer 不能自行归档，clean 后仍须独立 QA 放行' <<<"$repair_prompt"
  rg -q '使用独立 QA 会话' <<<"$qa_prompt"
  printf 'dynamic_quality_loop=passed\n' >>"$RUNTIME_LOG"
}

mkdir -p "$EVIDENCE_ROOT"
record_runtime
generate_default_config
verify_dynamic_contract
