#!/usr/bin/env bash
# 文件功能目的：验证 status/watch/JSON 状态展示已统一到共享 view/render 层。
# Sources: 23-统一状态展示视图模型, 27-统一状态展示视图入口, 31-拆分status-view渲染边界
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
LOG="$repo_root/test-results/refactor-stability/status-view-render-contract.log"
mkdir -p "$(dirname "$LOG")"
: >"$LOG"

note() {
  # note 记录测试关键步骤，便于定位共享状态视图合同失败点。
  printf '%s\n' "$*" | tee -a "$LOG"
}

cd "$repo_root"

note "evidence id: status-view-render-contract-log"
note "evidence path: $LOG"
note "test path: tests/specs/codex-workflow-cli/test_status_view_render_contract.sh"

if ! fd 'status.*render.*\.go|render.*status.*\.go' internal/app | tee -a "$LOG" | grep -q .; then
  note "internal/app 缺少 status render 源文件"
  exit 1
fi

[[ -f internal/app/status_view.go ]] || {
  note "internal/app 缺少共享 status view 源文件"
  exit 1
}

assert_file_has() {
  # assert_file_has 校验 status view 的内部职责边界落在约定源文件。
  local file="$1"
  local pattern="$2"
  [[ -f "$file" ]] || {
    note "缺少目标文件：$file"
    exit 1
  }
  rg -n "$pattern" "$file" >>"$LOG" || {
    note "$file 缺少模式：$pattern"
    exit 1
  }
}

note "检查 status view 内部职责边界"
assert_file_has "internal/app/status_view_model.go" 'func buildStatusView|func buildHumanStatusView'
assert_file_has "internal/app/status_duration.go" 'func stageDurationItems|func statusWorkflowWallDuration'
assert_file_has "internal/app/status_render_compact.go" 'func compactStatusLines|func statusDisplayWidth'
assert_file_has "internal/app/status_stale.go" 'func humanDisplayState|func isStaleRunningRun'

line_count="$(wc -l < internal/app/status_view.go | tr -d ' ')"
note "status_view.go line_count=$line_count"
if (( line_count > 350 )); then
  note "status_view.go 仍然过大，说明渲染边界没有真正拆分"
  exit 1
fi

if rg -n "func watchBatchStatusLines|func watchRunStatusLines|func runProposalStatusLines|func watchStageChecklistLines" internal/app/app.go | tee -a "$LOG" | grep -q .; then
  note "app.go 仍定义 watch/status 文本渲染 helper，展示职责尚未迁出"
  exit 1
fi

for symbol in \
  'func stageChecklistLines' \
  'func stageChecklistLinesWithParallel' \
  'func stageChecklistLinesForRepo' \
  'func stageDurationSummaryLines' \
  'func stageDurationItems' \
  'func visibleSessionItems' \
  'func plannerSessionID' \
  'func sessionRoleID' \
  'func roleOccurred'
do
  if rg -n "$symbol" internal/app/app.go | tee -a "$LOG" | grep -q .; then
    note "app.go 仍直接定义旧状态展示计算 helper：$symbol"
    exit 1
  fi
done

go test ./internal/app -run 'TestStatusView|TestPrintHumanStatus|TestWatch|TestRunner|TestCompactStatus|TestHumanStatus' -count=1 2>&1 | tee -a "$LOG"
