#!/usr/bin/env bash
# 文件目的：验证 sealed run 的阶段跳转规则由独立 stage decision 层表达并可回归测试。
# Sources: 22-抽离工作流状态机决策
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
log="$repo_root/test-results/refactor-stability/stage-decision-contract.log"
mkdir -p "$(dirname "$log")"
: >"$log"

# note 把关键断言同时写入 stdout 和 runtime log，方便 QA 定位失败点。
note() {
  printf '%s\n' "$*" | tee -a "$log"
}

# fail 用统一格式终止测试，避免结构断言静默失败。
fail() {
  note "FAIL: $*"
  exit 1
}

cd "$repo_root"
note "evidence id: stage-decision-contract-log"
note "evidence path: $log"
note "test path: tests/specs/codex-workflow-cli/test_stage_decision_contract.sh"

decision_files="$(fd -t f '.*stage.*decision.*\.go|.*decision.*stage.*\.go' internal/app || true)"
printf '%s\n' "$decision_files" | tee -a "$log"
if [[ -z "$decision_files" ]]; then
  fail "internal/app 缺少独立 stage decision 源文件"
fi

decision_symbols="$(rg -n 'type StageDecision|func DecideNextStage|func .*Stage.*Decision' internal/app || true)"
printf '%s\n' "$decision_symbols" | tee -a "$log"
if [[ -z "$decision_symbols" ]]; then
  fail "缺少 StageDecision 类型或下一阶段决策函数"
fi

state_lines="$(wc -l < internal/app/state.go)"
note "internal/app/state.go lines: $state_lines"
if [[ "$state_lines" -gt 1900 ]]; then
  fail "state.go 仍超过 1900 行，状态机职责尚未实质迁出"
fi

go test ./internal/app -run 'TestEngineStartRunsCleanReviewsToDone|TestQAFailureReturnsToFix|TestGoDAG|TestValidationGate|TestRootArtifactGate|TestRootAcceptancePreflight|TestEngineBlocksAfterLastFix|TestWorkflowFailureReviewFailsWorkflow' -count=1 2>&1 | tee -a "$log"
