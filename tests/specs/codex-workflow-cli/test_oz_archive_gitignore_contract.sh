#!/usr/bin/env bash
# 文件功能目的：验证 oz-archive 把 git 忽略规则作为不可绕过的提交边界。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
SKILL="$ROOT/skills/oz-archive/SKILL.md"

fail() {
  # 函数目的：报告归档技能缺失的忽略文件提交门禁。
  printf '[oz-archive-gitignore] FAIL: %s\n' "$*" >&2
  exit 1
}

assert_file_has() {
  # 函数目的：确认归档技能包含指定硬门禁表达。
  local pattern="$1"
  rg -n "$pattern" "$SKILL" >/dev/null || fail "缺少模式：$pattern"
}

[[ -f "$SKILL" ]] || fail "缺少技能文件：$SKILL"
assert_file_has '不得使用 `git add \.`、`git add -A` 或 `git add -f`'
assert_file_has '`git diff --cached --name-only`'
assert_file_has '`git check-ignore --no-index -- <path>`'
assert_file_has '不包含任何被 `.gitignore` 命中的路径'

printf '[oz-archive-gitignore] contract passed\n'
