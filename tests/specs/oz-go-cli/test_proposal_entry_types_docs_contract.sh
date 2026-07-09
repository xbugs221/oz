#!/usr/bin/env bash
# 文件功能目的：长期验证 README、规格和内置 oz skills 已说明 micro、small、standard 三种提案入口。
# Sources: 43-支持三种提案入口
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
LOG="$ROOT/test-results/specs/oz-go-cli/proposal-entry-types-docs-contract.log"

mkdir -p "$(dirname "$LOG")"
: >"$LOG"

note() {
  # 函数目的：记录文档合同检查步骤，便于定位缺失的入口规则。
  printf '[proposal-entry-types-docs] %s\n' "$*" | tee -a "$LOG"
}

fail() {
  # 函数目的：用明确业务原因终止测试，避免只暴露 rg 失败。
  note "FAIL: $*"
  exit 1
}

assert_file_has() {
  # 函数目的：确认目标文件包含指定正则模式。
  local file="$1"
  local pattern="$2"
  [[ -f "$file" ]] || fail "缺少文件：$file"
  rg -n "$pattern" "$file" >>"$LOG" || fail "$file 缺少模式：$pattern"
}

assert_file_lacks() {
  # 函数目的：确认目标文件不再保留和 small 入口冲突的强制规则。
  local file="$1"
  local pattern="$2"
  [[ -f "$file" ]] || fail "缺少文件：$file"
  if rg -n "$pattern" "$file" >>"$LOG"; then
    fail "$file 仍包含冲突规则：$pattern"
  fi
}

note "README 必须给出三种入口和 small 最小产物"
assert_file_has "$ROOT/README.md" 'micro.*TDD.*git commit|TDD.*git commit.*micro'
assert_file_has "$ROOT/README.md" 'small.*brief\.md.*acceptance\.json.*tests|brief\.md.*acceptance\.json.*tests.*small'
assert_file_has "$ROOT/README.md" 'standard.*proposal\.md.*design\.md.*spec\.md.*task\.md|完整提案.*standard'
assert_file_has "$ROOT/README.md" 'small.*长期规格|长期规格.*small'
assert_file_has "$ROOT/README.md" 'small.*(最多|<=).*2.*验收场景|small.*2.*required'
assert_file_has "$ROOT/README.md" 'standard.*升级触发器|升级 standard'
assert_file_has "$ROOT/README.md" '不得.*凑.*测试|不能.*凑.*任务|不.*硬凑'

note "长期规格必须记录 small validate 与归档责任"
assert_file_has "$ROOT/docs/specs/oz-go-cli.md" 'small.*brief\.md.*acceptance\.json.*tests|brief-only'
assert_file_has "$ROOT/docs/specs/oz-go-cli.md" 'micro.*TDD.*git commit|standard.*完整提案'
assert_file_has "$ROOT/docs/specs/oz-go-cli.md" 'small.*归档|归档.*small'
assert_file_has "$ROOT/docs/specs/oz-go-cli.md" 'small.*2.*验收场景|small.*2.*required'
assert_file_has "$ROOT/docs/specs/oz-go-cli.md" 'standard.*升级触发器|升级 standard'
assert_file_has "$ROOT/docs/specs/oz-go-cli.md" '不得.*凑.*测试|不能.*凑.*任务|不.*硬凑'

note "内置 skills 必须说明三种入口的职责边界"
assert_file_has "$ROOT/skills/oz-plan/SKILL.md" 'micro.*small.*standard|standard.*small.*micro'
assert_file_has "$ROOT/skills/oz-plan/SKILL.md" 'small.*2.*验收场景|small.*2.*required'
assert_file_has "$ROOT/skills/oz-plan/SKILL.md" '升级 standard|standard.*升级触发器'
assert_file_has "$ROOT/skills/oz-create/SKILL.md" 'small.*brief\.md.*acceptance\.json.*tests|brief-only'
assert_file_has "$ROOT/skills/oz-create/SKILL.md" 'standard.*proposal\.md.*design\.md.*spec\.md.*task\.md|完整提案'
assert_file_has "$ROOT/skills/oz-create/SKILL.md" '不得.*凑.*测试|不能.*凑.*任务|不.*硬凑'
assert_file_has "$ROOT/skills/oz-exec/SKILL.md" 'small.*brief\.md.*acceptance\.json.*tests|brief-only'
assert_file_has "$ROOT/skills/oz-archive/SKILL.md" 'small.*brief\.md|brief-only.*归档'

note "oz-create 不得继续表达为所有变更都必须六件套"
assert_file_lacks "$ROOT/skills/oz-create/SKILL.md" '创建 `oz` 变更提案并生成所有产物'

note "PASS: proposal-entry-types-docs-contract"
