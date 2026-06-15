#!/usr/bin/env bash
# 文件功能目的：运行当前 Go 回归测试，证明历史垃圾清理没有破坏真实 CLI 和包行为。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/36-cleanup"
LOG="$RESULT_DIR/go-test-all.log"

note() {
  # note 记录 Go 回归测试入口和结果，作为验收证据。
  printf '[go-test-all] %s\n' "$*" | tee -a "$LOG"
}

fail() {
  # fail 报告 Go 回归失败。
  note "FAIL: $*"
  exit 1
}

mkdir -p "$RESULT_DIR"
: >"$LOG"

cd "$ROOT"

note "运行 go test ./..."
go test ./... -count=1 2>&1 | tee -a "$LOG" || fail "go test ./... 未通过"

note "PASS"
