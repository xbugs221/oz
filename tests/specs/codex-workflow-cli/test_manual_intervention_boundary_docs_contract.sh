#!/usr/bin/env bash
# 文件功能目的：验证主规格把人工干预保护描述为路径感知规则，而不是禁止一切工作区变化。
# Sources: 16-允许运行中追加新需求但保留subagent写保护
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
LOG="$ROOT/test-results/manual-intervention-boundary-docs.log"
SPEC="$ROOT/docs/specs/codex-workflow-cli/spec.md"

note() {
  # 函数目的：记录文档门禁检查步骤。
  printf '[manual-intervention-docs] %s\n' "$*" | tee -a "$LOG"
}

fail() {
  # 函数目的：报告规格缺失的业务承诺并终止测试。
  printf '[manual-intervention-docs] FAIL: %s\n' "$*" | tee -a "$LOG" >&2
  exit 1
}

mkdir -p "$(dirname "$LOG")"
: >"$LOG"

note "主规格必须说明允许运行中新增非当前需求提案"
grep -Eiq '运行中.*(新增|追加).*(非当前|其他).*docs/changes|docs/changes/<非当前' "$SPEC" || fail "主规格没有声明运行中可新增非当前 docs/changes 提案"

note "主规格必须说明当前 change、源码和配置变化仍会中止"
grep -Eiq '当前.*change.*(源码|配置).*(中止|阻断)|源码.*配置.*当前.*change.*(中止|阻断)' "$SPEC" || fail "主规格没有保留当前 run 相关路径保护"

note "主规格必须说明 subagent 仍保持只读写保护"
grep -Eiq 'subagent.*只读|只读.*subagent|subagent.*写保护' "$SPEC" || fail "主规格没有说明 subagent 写保护"

note "contract passed: manual intervention boundary is documented"
