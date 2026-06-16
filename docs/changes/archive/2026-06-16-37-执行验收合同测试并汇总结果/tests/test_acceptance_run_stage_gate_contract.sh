#!/usr/bin/env bash
# 文件功能目的：验证 acceptance run 拥有独立边界，并接入 execution/fix 后的 deterministic gate。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/37-执行验收合同测试并汇总结果/gate"
LOG="$RESULT_DIR/contract.log"

fail() {
  printf 'FAIL: %s\n' "$*" | tee -a "$LOG" >&2
  exit 1
}

note() {
  printf '%s\n' "$*" | tee -a "$LOG"
}

rm -rf "$RESULT_DIR"
mkdir -p "$RESULT_DIR"
cd "$ROOT"

note "check independent acceptance run source boundary"
test -f internal/app/acceptance_run.go || fail "missing internal/app/acceptance_run.go"
rg -n 'type AcceptanceRunResult|type acceptanceRunResult' internal/app/acceptance_run.go >>"$LOG" || fail "missing acceptance run result DTO"
rg -n 'runAcceptance.*RequiredTests|runRequiredAcceptanceTests|runAcceptanceTests' internal/app/acceptance_run.go >>"$LOG" || fail "missing required_tests execution function"
rg -n 'required_evidence|Evidence' internal/app/acceptance_run.go >>"$LOG" || fail "missing required evidence status check"

note "check CLI and runner contract wiring"
rg -n 'run-acceptance' internal/app/app.go internal/app/command_dispatch.go internal/app/runner_contract.go >>"$LOG" || fail "run-acceptance not wired into CLI surface and runner contract"

note "check stage gate state is durable"
rg -n 'AcceptanceRun|acceptance_run|acceptanceRun' internal/app/state_model.go internal/app/validation.go internal/app/engine_stage.go internal/app/engine_run.go >>"$LOG" || fail "missing durable acceptance run gate state"
rg -n 'result_path|LastResult|last_result|LastArtifact' internal/app/acceptance_run.go internal/app/state_model.go internal/app/validation.go >>"$LOG" || fail "missing result path in acceptance run state"

note "check validation and QA boundaries do not own required_tests command execution"
if rg -n 'RequiredTests.*exec|exec\.Command.*RequiredTests|for .*RequiredTests' internal/app/validation.go internal/app/qa.go >>"$LOG"; then
  fail "validation.go or qa.go appears to own required_tests command execution"
fi

note "check Go regression tests exist and pass"
rg -n 'Test.*RunAcceptance|Test.*AcceptanceRun' internal/app >>"$LOG" || fail "missing Go regression tests for acceptance run command and gate"
go test ./internal/app -run 'Test.*RunAcceptance|Test.*AcceptanceRun' -count=1 >>"$LOG" 2>&1 || fail "acceptance run Go regressions failed"

note "gate contract passed; evidence: test-results/37-执行验收合同测试并汇总结果/gate/contract.log"

