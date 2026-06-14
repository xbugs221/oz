#!/usr/bin/env bash
# 文件功能目的：长期验证 oz flow 命令分发、交互流程和规划入口保持独立源码边界。
# Sources: 28-拆分wo命令分发边界
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
log="$repo_root/test-results/refactor-command-dispatch/oz-flow-command-dispatch-boundary-contract.log"
mkdir -p "$(dirname "$log")"
: >"$log"

note() {
  # note 记录关键步骤，同时产出 oz-flow-command-dispatch-boundary-log 证据。
  printf '%s\n' "$*" | tee -a "$log"
}

fail() {
  # fail 用明确命令边界原因终止测试。
  note "FAIL: $*"
  exit 1
}

cd "$repo_root"

note "evidence id: oz-flow-command-dispatch-boundary-log"
note "evidence path: $log"
note "test id: oz-flow-command-dispatch-boundary-contract"

for file in \
  internal/app/command_dispatch.go \
  internal/app/interactive.go \
  internal/app/planning.go
do
  [[ -f "$file" ]] || fail "缺少命令分发边界文件：$file"
  note "已发现边界文件：$file"
done

for command_case in \
  'case "run":' \
  'case "resume":' \
  'case "batch":' \
  'case "restart":' \
  'case "status":' \
  'case "abort":' \
  'case "clean":' \
  'case "watch":' \
  'case "--resume":' \
  'case "--run":'
do
  if rg -n "$command_case" internal/app/app.go | tee -a "$log" | grep -q .; then
    fail "app.go 仍直接包含 repo 命令 case：$command_case"
  fi
done

note "运行 internal/app 与 cmd/oz 命令面回归"
go test ./internal/app ./cmd/oz -count=1 2>&1 | tee -a "$log"

note "PASS: oz-flow-command-dispatch-boundary-contract"
