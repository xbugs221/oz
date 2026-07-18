#!/usr/bin/env bash
# 文件目的：验证 acceptance evidence producer 追溯逻辑已集中到 internal/acceptance。
set -euo pipefail

LOG="test-results/refactor-stability/shared-producer-contract.log"
mkdir -p "$(dirname "$LOG")"
: >"$LOG"
echo "evidence id: shared-producer-contract-log" | tee -a "$LOG"
echo "evidence path: $LOG" | tee -a "$LOG"
echo "test path: docs/changes/archive/2026-06-14-21-共享验收证据追溯逻辑/tests/shared-producer-contract_test.sh" | tee -a "$LOG"

if ! rg -n "func .*Producer.*(Finding|Evidence|Has)|func .*Evidence.*Producer" internal/acceptance | tee -a "$LOG" | grep -q .; then
  echo "internal/acceptance 缺少 producer 追溯共享 API" | tee -a "$LOG"
  exit 1
fi

if rg -n "func acceptance(EvidenceHasProducer|TestMentionsEvidence|TestScriptProducesEvidence|ProducerCandidatePaths|ProducerScriptMentionsTest|EvidenceNeedles|TextMentionsAny)|func stringSliceContains" cmd/oz internal/app | tee -a "$LOG" | grep -q .; then
  echo "cmd/oz 或 internal/app 仍定义本地 acceptance producer 追溯 helper" | tee -a "$LOG"
  exit 1
fi

go test ./internal/acceptance ./internal/app ./cmd/oz -count=1 2>&1 | tee -a "$LOG"
