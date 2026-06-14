#!/usr/bin/env bash
# 文件功能目的：验证 status/watch/runner JSON 展示计算统一到共享 status view/render 边界。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
log="$repo_root/test-results/refactor-status-view/status-view-unification-contract.log"
mkdir -p "$(dirname "$log")"
: >"$log"

note() {
  # note 记录关键步骤，同时产出 status-view-unification-log 证据。
  printf '%s\n' "$*" | tee -a "$log"
}

fail() {
  # fail 输出用户可理解的展示边界失败原因。
  note "FAIL: $*"
  exit 1
}

cd "$repo_root"

note "evidence id: status-view-unification-log"
note "evidence path: $log"
note "test id: status-view-unification-contract"

[[ -f internal/app/status_view.go ]] || fail "缺少共享 status view 文件"
[[ -f internal/app/status_render.go ]] || fail "缺少共享 status render 文件"

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
  if rg -n "$symbol" internal/app/app.go | tee -a "$log" | grep -q .; then
    fail "app.go 仍直接定义旧展示 helper：$symbol"
  fi
done

note "运行 status view、watch、human status 和 runner JSON 回归"
go test ./internal/app \
  -run 'TestStatusView|TestPrintHumanStatus|TestWatch|TestRunner|TestCompactStatus|TestHumanStatus' \
  -count=1 2>&1 | tee -a "$log"

note "PASS: status-view-unification-contract"
