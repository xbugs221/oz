#!/usr/bin/env bash
# 文件目的：验证 status/watch/JSON 状态展示已统一到共享 view/render 层。
set -euo pipefail

LOG="test-results/refactor-stability/status-view-contract.log"
mkdir -p "$(dirname "$LOG")"
: >"$LOG"
echo "evidence id: status-view-contract-log" | tee -a "$LOG"
echo "evidence path: $LOG" | tee -a "$LOG"
echo "test path: docs/changes/23-统一状态展示视图模型/tests/status-view-contract_test.sh" | tee -a "$LOG"

if ! fd 'status.*render.*\.go|render.*status.*\.go' internal/app | tee -a "$LOG" | grep -q .; then
  echo "internal/app 缺少 status render 源文件" | tee -a "$LOG"
  exit 1
fi

if rg -n "func watchBatchStatusLines|func watchRunStatusLines|func runProposalStatusLines|func watchStageChecklistLines" internal/app/app.go | tee -a "$LOG" | grep -q .; then
  echo "app.go 仍定义 watch/status 文本渲染 helper，展示职责尚未迁出" | tee -a "$LOG"
  exit 1
fi

go test ./internal/app -run 'TestStatusView|TestPrintHumanStatus|TestWatch|TestRunner|TestCompactStatus|TestHumanStatus' -count=1 2>&1 | tee -a "$LOG"
