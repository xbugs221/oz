#!/usr/bin/env bash
# 文件功能目的：以真实 internal/app 引擎测试验证全量自查、QA 定向修复、无固定轮次上限和可恢复阻塞。

set -euo pipefail

REPO_ROOT="$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel)"
cd "$REPO_ROOT"

EVIDENCE_ROOT="$REPO_ROOT/test-results/45-quality-delivery-loop"
RUNTIME_LOG="$EVIDENCE_ROOT/runtime.log"
PRE_QA_STATE="$EVIDENCE_ROOT/pre-qa-state.json"
TARGETED_STATE="$EVIDENCE_ROOT/targeted-repair-state.json"
UNBOUNDED_STATE="$EVIDENCE_ROOT/unbounded-state.json"
ENVIRONMENT_STATE="$EVIDENCE_ROOT/environment-resume-state.json"
STALLED_STATE="$EVIDENCE_ROOT/stalled-state.json"

BEHAVIOR_TESTS=(
  TestPreQAFullAuditRequiresCleanConfirmation
  TestQAFailureTargetsFindingsAndSelfValidates
  TestRepeatedQAFindingWithRepairProgressContinues
  TestQualityLoopQASessionsAreIsolated
  TestQualityLoopArchiveContextIncludesDynamicArtifacts
  TestRepairLoopHasNoIterationCeiling
  TestRepairEnvironmentBlockResumesWithoutBudget
  TestRepairStalledBlockResumesWithProgress
  TestRepairStalledBlockResumesWithEvidenceProgress
  TestQualityLoopPromptUsesRealDynamicArtifacts
  TestQualityLoopValidationUsesProgressInsteadOfAttemptLimit
  TestQualityLoopAcceptanceUsesSealedContract
  TestQualityLoopMissingSealedAcceptanceFailsClosed
  TestQualityLoopTargetedRepairRequiresSourceQA
  TestQualityLoopAcceptanceEnvironmentMarkerBlocks
  TestQualityLoopRepairArtifactEnvironmentMarkerBlocks
  TestQualityLoopGateSnapshotRejectsPostTestDiff
  TestQualityLoopGateStallUsesGateProgress
  TestQualityProgressHashIgnoresVolatileGateArtifacts
  TestQualityFailureFingerprintsIgnoreVolatileLogs
  TestQualityLoopEvidenceContentChangeCountsAsProgress
  TestQualityEvidenceProgressHashStreamsLargeArtifacts
  TestVerifyQualityAcceptanceCheckpointAcceptsLegacyProgressHash
  TestQualityLoopResumeChecksLockBeforeUnblocking
  TestQualityLoopBatchPreservesRecoverableRun
  TestQualityLoopManualRestartResumesStalledRun
  TestQualityLoopRetryPromptKeepsTargetedScope
  TestNeedsFixQAMatrixMustCoverSealedContract
  TestGitChangeContentSnapshotTracksEffectiveContent
  TestStalledResumeRejectsCommitOfExistingDiff
  TestQABlockedResumeWithSourceProgressRoutesToTargetedRepair
  TestQABlockedResumeWithUntrustedArtifactRoutesToFreshAudit
  TestQualityLoopQAReadOnlyGateBlocksCommittedSourceMutation
  TestQualityLoopQAGateRejectsCheckpointDriftDuringRun
  TestQualityLoopArchiveGateAllowsMoveButRejectsSourceMutation
  TestQualityLoopArchiveGateRejectsFinalQAContentDrift
)

# behavior_test_pattern 返回严格锚定的 Go 测试过滤器，避免零匹配被误判为通过。
behavior_test_pattern() {
  local joined
  joined="$(IFS='|'; printf '%s' "${BEHAVIOR_TESTS[*]}")"
  printf '^(%s)$' "$joined"
}

# reset_evidence 删除上轮产物，确保本轮只能消费当前测试新生成的证据。
reset_evidence() {
  rm -rf "$EVIDENCE_ROOT"
  mkdir -p "$EVIDENCE_ROOT"
}

# require_behavior_tests 通过 go test -list 精确确认每个行为测试已注册。
require_behavior_tests() {
  local listed test_name
  listed="$(go test ./internal/app -list "$(behavior_test_pattern)")"
  for test_name in "${BEHAVIOR_TESTS[@]}"; do
    rg -qx --fixed-strings "$test_name" <<<"$listed"
  done
}

# require_source_contract 核对新状态与模式已进入真实生产状态机。
require_source_contract() {
  rg -q 'pre_qa_audit' internal/app
  rg -q 'qa_targeted_repair' internal/app
  rg -q 'blocked_environment' internal/app
  rg -q 'blocked_stalled' internal/app
}

# run_engine_contracts 执行真实引擎集成测试并生成可由 QA 独立复核的状态证据。
run_engine_contracts() {
  QUALITY_PRE_QA_STATE="$PRE_QA_STATE" \
    QUALITY_TARGETED_STATE="$TARGETED_STATE" \
    QUALITY_UNBOUNDED_STATE="$UNBOUNDED_STATE" \
    QUALITY_ENVIRONMENT_STATE="$ENVIRONMENT_STATE" \
    QUALITY_STALLED_STATE="$STALLED_STATE" \
    go test -v ./internal/app \
      -run "$(behavior_test_pattern)" \
      -count=1 2>&1 | tee "$RUNTIME_LOG"
}

# verify_runtime_tests 核对本轮日志确实运行并通过了全部行为测试。
verify_runtime_tests() {
  local test_name
  for test_name in "${BEHAVIOR_TESTS[@]}"; do
    rg -q --fixed-strings -- "=== RUN   $test_name" "$RUNTIME_LOG"
    rg -q --fixed-strings -- "--- PASS: $test_name " "$RUNTIME_LOG"
  done
}

# verify_evidence 核对关键业务结果，防止只生成空文件或无意义状态。
verify_evidence() {
  test -s "$RUNTIME_LOG"
  test -s "$PRE_QA_STATE"
  test -s "$TARGETED_STATE"
  test -s "$UNBOUNDED_STATE"
  test -s "$ENVIRONMENT_STATE"
  test -s "$STALLED_STATE"

  jq -e '
    .repairs.audit_1.decision == "needs_more" and
    .repairs.audit_2.decision == "clean" and
    .states.final.stages.audit_1 == "completed" and
    .states.final.stages.audit_2 == "completed" and
    .states.final.acceptance_run.audit_2.status == "passed" and
    (.acceptance_results.audit_2.tests | length) > 0 and
    (.acceptance_results.audit_2.tests | all(.status == "passed")) and
    .states.final.stages.qa_1 == "completed" and
    .qas.qa_1.decision == "needs_fix"
  ' "$PRE_QA_STATE" >/dev/null

  jq -e '
    .states.final.quality_loop.mode == "qa_targeted_repair" and
    (.states.final.quality_loop.source_qa_artifact | endswith("qa-2.json")) and
    (.states.final.quality_loop.source_qa_hash | length) > 0 and
    .states.final.artifact_gates.qa_1.kind == "qa_read_only" and
    .states.final.artifact_gates.qa_1.status == "passed" and
    (.states.final.artifact_gates.qa_1.checkpoint_hash | length) > 0 and
    .states.final.artifact_gates.qa_2.kind == "qa_read_only" and
    .states.final.artifact_gates.qa_2.status == "passed" and
    (.states.final.artifact_gates.qa_2.checkpoint_hash | length) > 0 and
    .qas.qa_1.decision == "needs_fix" and
    .repairs.targeted_repair_1.decision == "clean" and
    .states.final.quality_loop.failed_tests_replayed == true and
    .states.final.acceptance_run.targeted_repair_1.status == "passed" and
    (.acceptance_results.targeted_repair_1.tests | length) > 0 and
    (.acceptance_results.targeted_repair_1.tests | all(.status == "passed")) and
    (.validation_attempts.targeted_repair_1.commands |
      any(.exit_code == 0 and (.output | contains("quality-loop-validation-ok")))) and
    (.states.final.validation.targeted_repair_1.diff_hash | length) > 0 and
    .states.final.validation.targeted_repair_1.diff_hash ==
      .states.final.acceptance_run.targeted_repair_1.diff_hash and
    .states.final.acceptance_run.targeted_repair_1.diff_hash ==
      .acceptance_results.targeted_repair_1.diff_hash and
    .states.failed_validation.status == "blocked_stalled" and
    .states.failed_validation.stage == "blocked_stalled" and
    .states.failed_validation.validation.targeted_repair_1.status == "failed" and
    .states.failed_validation.stages.qa_2 == null and
    .states.final.stages.qa_2 == "completed" and
    .qas.qa_2.decision == "clean"
  ' "$TARGETED_STATE" >/dev/null

  jq -e '
    .states.final.status == "done" and
    .states.final.stage == "done" and
    .states.final.stages.targeted_repair_12 == "completed" and
    .states.final.stages.qa_13 == "completed" and
    ([.qas | to_entries[] |
      select(.key != "qa_13") |
      .value |
      select(.decision == "needs_fix")] | length) == 12 and
    .qas.qa_13.decision == "clean" and
    ([.states.final.stages | to_entries[] |
      select(.key == "blocked_review_limit" or .value == "blocked_review_limit")] | length) == 0
  ' "$UNBOUNDED_STATE" >/dev/null

  jq -e '
    .states.blocked.status == "blocked_environment" and
    .states.blocked.stage == "blocked_environment" and
    .states.blocked.quality_loop.blocked_from_stage == "audit_3" and
    .states.blocked.quality_loop.missing_environment_names ==
      ["environment/available-after-resume"] and
    .states.resumed.status == "running" and
    .states.resumed.stage == "audit_3" and
    .states.resumed.quality_loop.resume_rerun_pending == true and
    .states.resumed.stages.audit_3 != "needs_more" and
    .states.rerun.stage == "audit_3" and
    .states.rerun.quality_loop.resume_rerun_pending != true
  ' "$ENVIRONMENT_STATE" >/dev/null
  if rg -q --fixed-strings 'do-not-record' "$ENVIRONMENT_STATE"; then
    return 1
  fi

  jq -e '
    .states.blocked.status == "blocked_stalled" and
    .states.blocked.stage == "blocked_stalled" and
    .states.blocked.quality_loop.blocked_from_stage == "qa_1" and
    (.states.blocked.quality_loop.finding_fingerprint | length) > 0 and
    (.states.blocked.quality_loop.progress_hash | length) > 0 and
    .states.resumed.status == "running" and
    .states.resumed.stage == "audit_1" and
    .states.resumed.quality_loop.mode == "pre_qa_audit" and
    .states.resumed.quality_loop.resume_rerun_pending == true and
    .states.resumed.stages.qa_1 != "needs_more" and
    .states.resumed.quality_loop.finding_fingerprint == null and
    .states.blocked.quality_loop.diff_hash !=
      .states.resumed.quality_loop.diff_hash and
    .qas.source.decision == "needs_fix"
  ' "$STALLED_STATE" >/dev/null
}

reset_evidence
require_behavior_tests
require_source_contract
run_engine_contracts
verify_runtime_tests
verify_evidence
