#!/usr/bin/env bash
# 文件功能目的：验证 oz flow status --run-id --json 在存在 parallel artifacts 时仍保持 runner 机器接口，不输出人类并行摘要。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/32-status-parallel/json"

rm -rf "$RESULT_DIR"
mkdir -p "$RESULT_DIR"
cd "$ROOT"

OZ_STATUS_PARALLEL_RESULT_DIR="$RESULT_DIR"   go test ./internal/app -run TestStatusJSONDoesNotExposeParallelHumanSummary -count=1 -v | tee "$RESULT_DIR/contract.log"
