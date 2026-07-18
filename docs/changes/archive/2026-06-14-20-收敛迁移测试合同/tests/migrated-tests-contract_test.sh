#!/usr/bin/env bash
# 文件目的：验证迁移测试合同已收敛，根测试门禁可作为后续重构稳定基线。
set -euo pipefail

LOG="test-results/refactor-stability/migrated-tests-contract.log"
mkdir -p "$(dirname "$LOG")"
: >"$LOG"
echo "evidence id: migrated-tests-contract-log" | tee -a "$LOG"
echo "evidence path: $LOG" | tee -a "$LOG"
echo "test path: docs/changes/archive/2026-06-14-20-收敛迁移测试合同/tests/migrated-tests-contract_test.sh" | tee -a "$LOG"

if find docs/changes/archive/2026-06-14-20-收敛迁移测试合同/tests/app -type f -name '*.gotest' -print | tee -a "$LOG" | grep -q .; then
  echo "docs/changes/archive/2026-06-14-20-收敛迁移测试合同/tests/app 仍存在 .gotest 迁移测试输入，必须先迁入真实 Go 测试包或删除过期合同" | tee -a "$LOG"
  exit 1
fi

go test ./internal/app ./cmd/oz ./tests -count=1 2>&1 | tee -a "$LOG"
go test ./... -count=1 2>&1 | tee -a "$LOG"
