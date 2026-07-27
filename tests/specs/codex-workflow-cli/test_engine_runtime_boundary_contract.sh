#!/usr/bin/env bash
# 文件功能目的：验证 workflow Engine 核心运行路径按职责拆分且回归行为稳定。
# Sources: 32-拆分工作流Engine运行边界
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
log="$repo_root/test-results/engine-runtime-boundary/contract.log"
mkdir -p "$(dirname "$log")"
: >"$log"

note() {
  # note 记录边界检查步骤并产出 engine-runtime-boundary-log 证据。
  printf '%s\n' "$*" | tee -a "$log"
}

fail() {
  # fail 用业务语义说明失败点，方便归档规格测试定位职责回退。
  note "FAIL: $*"
  exit 1
}

assert_file_has() {
  # assert_file_has 确认目标文件存在且承载预期的 Engine 职责符号。
  local file="$1"
  local pattern="$2"
  [[ -f "$file" ]] || fail "缺少 Engine 运行边界文件：$file"
  rg -n "$pattern" "$file" >>"$log" || fail "$file 缺少模式：$pattern"
  note "已验证边界文件：$file"
}

cd "$repo_root"

note "evidence id: engine-runtime-boundary-log"
note "evidence path: $log"
note "test id: engine-runtime-boundary-contract"

assert_file_has "internal/app/state_model.go" 'type State struct|type DAGNodeState struct'
assert_file_has "internal/app/engine_run.go" 'func NewEngine|func \(e \*Engine\) runLoop'
assert_file_has "internal/app/engine_resume.go" 'func \(e \*Engine\) Resume|func \(e \*Engine\) resumeRun'
assert_file_has "internal/app/engine_stage.go" 'func \(e \*Engine\) runStage|func \(e \*Engine\) stageOptionsForRun'
assert_file_has "internal/app/engine_progress.go" 'type stageProgressWriter|func persistStateSessionID'

note "state.go 以职责符号归属校验，不再用后续需求会自然突破的行数阈值判断边界"

note "运行 internal/app workflow 相关 Go 回归"
go test ./internal/app -run 'Test(GoDAG|GateState|Migrated|Restart|StageDecision|Status)' -count=1 2>&1 | tee -a "$log"

note "PASS: engine-runtime-boundary-contract"
