#!/usr/bin/env bash
# 文件功能：验证 status view 重构后的职责边界和现有回归测试仍然成立。

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
cd "$ROOT"

EVIDENCE="test-results/31-status-view-boundary/contract.log"
mkdir -p "$(dirname "$EVIDENCE")"
: > "$EVIDENCE"

note() {
  printf '%s\n' "$*" | tee -a "$EVIDENCE"
}

fail() {
  note "FAIL: $*"
  exit 1
}

assert_file_has() {
  local file="$1"
  local pattern="$2"
  [[ -f "$file" ]] || fail "缺少目标文件：$file"
  rg -n "$pattern" "$file" >>"$EVIDENCE" || fail "$file 缺少模式：$pattern"
}

note "status-view-boundary-log: 检查 status view 文件边界"
assert_file_has "internal/app/status_view_model.go" 'func buildStatusView|func buildHumanStatusView'
assert_file_has "internal/app/status_duration.go" 'func stageDurationItems|func statusWorkflowWallDuration'
assert_file_has "internal/app/status_render_compact.go" 'func compactStatusLines|func statusDisplayWidth'
assert_file_has "internal/app/status_stale.go" 'func humanDisplayState|func isStaleRunningRun'

if [[ -f internal/app/status_view.go ]]; then
  line_count="$(wc -l < internal/app/status_view.go | tr -d ' ')"
  note "status_view.go line_count=$line_count"
  (( line_count <= 350 )) || fail "status_view.go 仍然过大，说明渲染边界没有真正拆分"
fi

note "运行 internal/app status/watch 相关 Go 回归"
go test ./internal/app -run 'Test(Status|RunningSubagent|StageDuration|Compact|Human)' -count=1 | tee -a "$EVIDENCE"

note "contract passed: status view 边界已拆分，证据位于 $EVIDENCE"
