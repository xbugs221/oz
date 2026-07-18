#!/usr/bin/env bash
# 文件目的：验证工作流状态机决策已从 state.go 抽离成可测试的独立层。
set -euo pipefail

LOG="test-results/refactor-stability/stage-decision-contract.log"
mkdir -p "$(dirname "$LOG")"
: >"$LOG"
echo "evidence id: stage-decision-contract-log" | tee -a "$LOG"
echo "evidence path: $LOG" | tee -a "$LOG"
echo "test path: docs/changes/archive/2026-06-14-22-抽离工作流状态机决策/tests/stage-decision-contract_test.sh" | tee -a "$LOG"

decision_files="$(find internal/app -type f \( -name '*stage*decision*.go' -o -name '*decision*stage*.go' \))"
printf '%s\n' "$decision_files" | tee -a "$LOG"
if [ -z "$decision_files" ]; then
  echo "internal/app 缺少独立 stage decision 源文件" | tee -a "$LOG"
  exit 1
fi

decision_symbols="$(rg -n "type StageDecision|func DecideNextStage|func .*Stage.*Decision" internal/app || true)"
printf '%s\n' "$decision_symbols" | tee -a "$LOG"
if [ -z "$decision_symbols" ]; then
  echo "缺少 StageDecision 类型或下一阶段决策函数" | tee -a "$LOG"
  exit 1
fi

state_lines="$(wc -l < internal/app/state.go)"
echo "internal/app/state.go lines: $state_lines" | tee -a "$LOG"
if [ "$state_lines" -gt 1900 ]; then
  echo "state.go 仍超过 1900 行，状态机职责尚未实质迁出" | tee -a "$LOG"
  exit 1
fi

go test ./internal/app -run 'TestEngineStartRunsCleanReviewsToDone|TestQAFailureReturnsToFix|TestGoDAG|TestValidationGate|TestRootArtifactGate|TestRootAcceptancePreflight|TestEngineBlocksAfterLastFix|TestWorkflowFailureReviewFailsWorkflow' -count=1 2>&1 | tee -a "$LOG"
