#!/usr/bin/env bash
# 文件功能：验证仓库不再依赖 shell 临时生成 .gotest 的动态测试机制。

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../../.." && pwd)"
cd "$ROOT"

EVIDENCE="test-results/35-gotest-migration/contract.log"
mkdir -p "$(dirname "$EVIDENCE")"
: > "$EVIDENCE"

note() {
  printf '%s\n' "$*" | tee -a "$EVIDENCE"
}

fail() {
  note "FAIL: $*"
  exit 1
}

note "gotest-migration-log: 检查动态 .gotest 机制是否已清除"

if [[ -f tests/app/migrated_app_suite_test.go ]]; then
  fail "tests/app/migrated_app_suite_test.go 仍存在，动态 runner 尚未删除"
fi

if matches="$(rg -n '\\.gotest|OZ_MIGRATED_APP_RUN|migrated_app_suite' tests/specs tests/app 2>/dev/null || true)" && [[ -n "$matches" ]]; then
  printf '%s\n' "$matches" >>"$EVIDENCE"
  fail "仍存在动态 .gotest 或 migrated runner 引用"
fi

if gotests="$(fd . tests/app -e gotest 2>/dev/null || true)" && [[ -n "$gotests" ]]; then
  printf '%s\n' "$gotests" >>"$EVIDENCE"
  fail "tests/app 仍存在 .gotest 文件"
fi

note "运行完整 Go 测试入口，确认迁移后没有隐藏动态测试依赖"
go test ./... | tee -a "$EVIDENCE"

note "contract passed: 动态 gotest 合同已迁移，证据位于 $EVIDENCE"
