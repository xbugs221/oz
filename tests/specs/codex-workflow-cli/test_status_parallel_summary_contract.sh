#!/usr/bin/env bash
# 文件功能目的：验证 oz flow status 人类输出不泄漏 parallel fan-in 摘要，同时机器 gate 仍严格校验 parallel artifact。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/32-status-parallel/summary"

rm -rf "$RESULT_DIR"
mkdir -p "$RESULT_DIR"
cd "$ROOT"

OZ_STATUS_PARALLEL_RESULT_DIR="$RESULT_DIR"   go test ./internal/app -run 'TestHumanStatusHidesParallelFanInSummary|TestParallelReviewGateAllowsMissingOrInvalidArtifact|TestParallelReviewGateAllowsUnconfiguredParallelMembers|TestBatchHumanStatusHidesParallelSummaryUnderChange' -count=1 -v | tee "$RESULT_DIR/contract.log"
