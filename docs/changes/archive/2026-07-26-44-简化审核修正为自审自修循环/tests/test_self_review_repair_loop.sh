#!/usr/bin/env bash
# 文件功能目的：验证 oz flow 已把 reviewer/fixer 往返简化为可恢复的同会话优化循环。

set -euo pipefail

REPO_ROOT="$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel)"
cd "$REPO_ROOT"
EVIDENCE_ROOT="$REPO_ROOT/test-results/44-self-review-repair-loop"
REPAIR_STATE_EVIDENCE="$EVIDENCE_ROOT/state.json"
QA_REPAIR_STATE_EVIDENCE="$EVIDENCE_ROOT/qa-repair-state.json"
REPAIR_LIMIT_STATE_EVIDENCE="$EVIDENCE_ROOT/repair-limit-state.json"
REPAIR_LEGACY_STATE_EVIDENCE="$EVIDENCE_ROOT/legacy-resume-state.json"
REPAIR_RUNTIME_EVIDENCE="$EVIDENCE_ROOT/runtime.log"

# require_source_contract 核对真实运行源码已注册优化角色与新阶段，而不是只修改提案文档。
require_source_contract() {
  rg -q 'Session: "repairer"' internal/app/stage_role.go
  rg -q 'repair_' internal/app/workflow_stage.go internal/app/stage_decision.go
  rg -q 'max_repair_iterations|MaxRepairIterations' internal/app
  rg -q '最大.*10|<= 10|> 10|MaxRepairIterations.*10' internal/app
}

# require_zero_round_contract 核对零轮 repair 的文档语义与独立 QA 门禁保持一致。
require_zero_round_contract() {
  rg -q '配置为 0 时禁用 repair' docs/changes/archive/2026-07-26-44-简化审核修正为自审自修循环/proposal.md
  rg -q '0 时禁用 repair，不生成 repair artifact，仅由独立 QA clean 放行' docs/changes/archive/2026-07-26-44-简化审核修正为自审自修循环/spec.md
  rg -q 'max_repair_iterations=0.*禁用 repair' docs/changes/archive/2026-07-26-44-简化审核修正为自审自修循环/design.md
  rg -q '零轮模式禁用 repair，仅由独立 QA clean 放行' docs/changes/archive/2026-07-26-44-简化审核修正为自审自修循环/task.md
}

# require_behavior_tests 核对实现层存在业务行为测试，避免用静态字符串满足状态机合同。
require_behavior_tests() {
  rg -q 'Test.*Repair.*Session|Test.*Repair.*Reuse' internal/app/*_test.go
  rg -q 'Test.*QA.*Repair|Test.*Repair.*QA' internal/app/*_test.go
  rg -q 'Test.*Repair.*Limit|Test.*Repair.*Legacy|Test.*Legacy.*Repair' internal/app/*_test.go
  rg -q 'TestRepairConfirmationFindingResetsPending|TestRepairCleanAtLimitBlocksWithoutConfirmation' internal/app/stage_decision_test.go
}

# run_targeted_tests 运行真实 app 包测试，覆盖阶段决策、artifact gate 与 sealed run 恢复路径。
run_targeted_tests() {
  mkdir -p "$EVIDENCE_ROOT"
  REPAIR_STATE_EVIDENCE="$QA_REPAIR_STATE_EVIDENCE" \
    REPAIR_LIMIT_STATE_EVIDENCE="$REPAIR_LIMIT_STATE_EVIDENCE" \
    REPAIR_LEGACY_STATE_EVIDENCE="$REPAIR_LEGACY_STATE_EVIDENCE" \
    go test -v ./internal/app -run 'Test.*(Repair|Legacy).*' -count=1 \
    2>&1 | tee "$REPAIR_RUNTIME_EVIDENCE"
  test -s "$QA_REPAIR_STATE_EVIDENCE"
  test -s "$REPAIR_LIMIT_STATE_EVIDENCE"
  test -s "$REPAIR_LEGACY_STATE_EVIDENCE"
  test -s "$REPAIR_RUNTIME_EVIDENCE"
  jq -s '{
    qa_repair_resume: .[0],
    repair_limit_blocked: .[1],
    legacy_review_fix_resume: .[2]
  }' \
    "$QA_REPAIR_STATE_EVIDENCE" \
    "$REPAIR_LIMIT_STATE_EVIDENCE" \
    "$REPAIR_LEGACY_STATE_EVIDENCE" > "$REPAIR_STATE_EVIDENCE.tmp"
  mv "$REPAIR_STATE_EVIDENCE.tmp" "$REPAIR_STATE_EVIDENCE"
  jq -e '
    .qa_repair_resume.sealed == true and
    .qa_repair_resume.status == "done" and
    .qa_repair_resume.sessions["codex:repairer"] == "repair-session" and
    .qa_repair_resume.sessions["codex:qa"] == "qa-session" and
    .qa_repair_resume.stages.repair_3 == "completed" and
    .qa_repair_resume.stages.qa_3 == "completed" and
    .qa_repair_resume.repair_confirmation_pending != true and
    .repair_limit_blocked.sealed == true and
    .repair_limit_blocked.status == "blocked_review_limit" and
    .repair_limit_blocked.stage == "blocked_review_limit" and
    (.repair_limit_blocked.error | contains("达到上限")) and
    .legacy_review_fix_resume.sealed == true and
    .legacy_review_fix_resume.workflow_config.max_review_iterations == 2 and
    .legacy_review_fix_resume.status == "done" and
    .legacy_review_fix_resume.stages.review_2 == "completed" and
    .legacy_review_fix_resume.stages.qa_2 == "completed"
  ' "$REPAIR_STATE_EVIDENCE" >/dev/null
  rg -q -- '--- PASS: TestRepairWorkflowDAGResumeEvidence' "$REPAIR_RUNTIME_EVIDENCE"
  rg -q -- '--- PASS: TestZeroRepairWorkflowDAGArchive' "$REPAIR_RUNTIME_EVIDENCE"
  rg -q -- '--- PASS: TestRepairLimitBlockedWorkflowEvidence' "$REPAIR_RUNTIME_EVIDENCE"
  rg -q -- '--- PASS: TestLegacyRepairWorkflowResumeEvidence' "$REPAIR_RUNTIME_EVIDENCE"
}

require_source_contract
require_zero_round_contract
require_behavior_tests
run_targeted_tests
