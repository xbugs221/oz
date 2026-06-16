#!/usr/bin/env bash
# Sources: 37-执行验收合同测试并汇总结果
# 文件功能目的：验证 oz flow run-acceptance 命令面、执行汇总和 sealed run gate 边界。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
CHANGE_DIR="$ROOT/docs/changes/archive/2026-06-16-37-执行验收合同测试并汇总结果/tests"
RESULT_DIR="$ROOT/test-results/specs/codex-workflow-cli/acceptance-run"
LOG="$RESULT_DIR/contract.log"

mkdir -p "$RESULT_DIR"
: >"$LOG"

for script in \
  test_acceptance_run_contract_surface.sh \
  test_acceptance_run_success_contract.sh \
  test_acceptance_run_failure_contract.sh \
  test_acceptance_run_stage_gate_contract.sh
do
  bash "$CHANGE_DIR/$script" >>"$LOG" 2>&1
done

printf 'contract passed: acceptance run command, result summary, evidence check, and gate boundary verified\n' | tee -a "$LOG"
