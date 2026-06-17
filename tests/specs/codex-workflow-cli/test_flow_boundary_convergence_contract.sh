#!/usr/bin/env bash
# 文件功能目的：验证 oz flow 命令入口、workflow topology 和公共 status view 边界已收敛。
# Sources: 41-收敛flow命令入口和状态映射
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
log="$repo_root/test-results/refactor-flow-boundary/flow-boundary-convergence-contract.log"
mkdir -p "$(dirname "$log")"
: >"$log"

note() {
  # note 同步输出测试进度，并产出 flow-boundary-convergence-log 运行证据。
  printf '%s\n' "$*" | tee -a "$log"
}

fail() {
  # fail 用业务级原因终止测试，避免执行阶段只看到 shell 行号。
  note "FAIL: $*"
  exit 1
}

require_file() {
  # require_file 确认目标重构边界已经成为生产代码文件。
  local path="$1"
  [[ -f "$repo_root/$path" ]] || fail "缺少目标边界文件：$path"
  note "已发现目标边界文件：$path"
}

reject_pattern() {
  # reject_pattern 禁止指定文件继续保留本次要消除的重复职责。
  local pattern="$1"
  shift
  if rg -n "$pattern" "$@" 2>/dev/null | tee -a "$log" | grep -q .; then
    fail "发现禁止的重复边界模式：$pattern"
  fi
}

cd "$repo_root"

note "evidence id: flow-boundary-convergence-log"
note "evidence path: $log"
note "test id: flow-boundary-convergence-contract"

require_file "internal/app/flow_command_registry.go"
require_file "internal/app/workflow_topology.go"

if ! rg -n 'type flowCommandSpec struct|var flowCommandRegistry|func flowCommandByName' internal/app/flow_command_registry.go | tee -a "$log" | grep -q .; then
  fail "flow_command_registry.go 必须定义命令 registry 结构或查询入口"
fi

reject_pattern 'func commandNeedsWorkflowTools|case "config":|case "run":|case "resume":|case "batch":|case "restart":|case "status":|case "abort":|case "clean":|case "watch":|case "--resume":|case "--run":' \
  internal/app/app.go

reject_pattern 'case "config":' \
  internal/app/command_dispatch.go

reject_pattern 'planning_context|implementation_context|before_execution|before_review|before_qa' \
  internal/app/graph.go \
  internal/app/status_view_model.go \
  internal/app/status_parallel.go \
  internal/app/config_parallel.go

reject_pattern 'stageChecklistLines|stageChecklistLinesWithParallel|stageChecklistLinesForRepo|watchStageChecklistLines' \
  internal/app/app.go \
  internal/app/engine_progress.go \
  internal/app/status_render.go

note "运行真实 Go 命令面和状态回归"
go test ./internal/app ./internal/ozcli ./tests -count=1 2>&1 | tee -a "$log"

note "PASS: flow-boundary-convergence-contract"
