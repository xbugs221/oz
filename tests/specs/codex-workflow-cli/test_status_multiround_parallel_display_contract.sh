#!/usr/bin/env bash
# Sources: 13-修正-oz-flow-status-多轮并行状态展示
# 文件功能目的：验证 oz flow status/watch 在多轮 review/fix 与 parallel subagent 并存时不泄漏内部 fan-in，并显示当前轮次状态。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/status-multiround-parallel-display"
LOG="$RESULT_DIR/status-multiround-parallel-display.log"

mkdir -p "$RESULT_DIR"
cd "$ROOT"

go test ./internal/app -run TestHumanStatusMultiRoundParallelDisplayContract -count=1 -v 2>&1 | tee "$LOG"
