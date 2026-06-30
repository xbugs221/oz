#!/usr/bin/env bash
# 文件功能目的：验证 README 和内置 oz skills 保留流程化、可验证的工作流表达。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
LOG="$ROOT/test-results/specs/codex-workflow-cli/skill-workflow-docs-contract.log"

mkdir -p "$(dirname "$LOG")"
: >"$LOG"

note() {
  # 函数目的：记录文档合同检查步骤，便于定位失败来源。
  printf '[skill-workflow-docs] %s\n' "$*" | tee -a "$LOG"
}

fail() {
  # 函数目的：报告缺失的文档合同并终止测试。
  printf '[skill-workflow-docs] FAIL: %s\n' "$*" | tee -a "$LOG" >&2
  exit 1
}

assert_file_has() {
  # 函数目的：确认文件包含指定正则模式。
  local file="$1"
  local pattern="$2"
  [[ -f "$file" ]] || fail "缺少文件：$file"
  rg -n "$pattern" "$file" >>"$LOG" || fail "$file 缺少模式：$pattern"
}

note "README 必须说明 skill、change、acceptance 和 flow 的职责边界"
assert_file_has "$ROOT/README.md" 'skill.*怎么做|skill.*作用'
assert_file_has "$ROOT/README.md" 'oz flow.*何时推进|flow.*门禁'
assert_file_has "$ROOT/README.md" 'acceptance\.json.*证据|acceptance.*可执行合同'
assert_file_has "$ROOT/README.md" 'change 目录|docs/changes/<change>/'

note "四个内置 oz skill 必须保留流程、退出条件和反偷懒检查"
for skill in oz-plan oz-create oz-exec oz-archive; do
  file="$ROOT/skills/$skill/SKILL.md"
  assert_file_has "$file" '^## 流程$'
  assert_file_has "$file" '^## 退出条件$'
  assert_file_has "$file" '^## 反偷懒检查$'
  assert_file_has "$file" '常见偷懒理由'
done

note "contract passed: workflow docs and skills stay process-oriented"
