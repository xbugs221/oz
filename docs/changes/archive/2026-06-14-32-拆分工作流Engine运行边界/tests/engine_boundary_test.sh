#!/usr/bin/env bash
# 文件功能：验证 Engine 运行边界拆分后仍保持核心 workflow 回归通过。

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
cd "$ROOT"

EVIDENCE="test-results/32-engine-boundary/contract.log"
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

note "engine-boundary-log: 检查 Engine 文件边界"
assert_file_has "internal/app/state_model.go" 'type State struct|type DAGNodeState struct'
assert_file_has "internal/app/engine_run.go" 'func NewEngine|func \(e \*Engine\) runLoop'
assert_file_has "internal/app/engine_resume.go" 'func \(e \*Engine\) Resume|func \(e \*Engine\) resumeRun'
assert_file_has "internal/app/engine_stage.go" 'func \(e \*Engine\) runStage|func \(e \*Engine\) stageOptionsForRun'
assert_file_has "internal/app/engine_progress.go" 'type stageProgressWriter|func persistStateSessionID'

if [[ -f internal/app/state.go ]]; then
  line_count="$(wc -l < internal/app/state.go | tr -d ' ')"
  note "state.go line_count=$line_count"
  (( line_count <= 260 )) || fail "state.go 仍然过大，说明 Engine 运行职责没有真正拆分"
fi

note "运行 internal/app workflow 相关 Go 回归"
go test ./internal/app -run 'Test(GoDAG|GateState|Migrated|Restart|StageDecision|Status)' -count=1 | tee -a "$EVIDENCE"

note "contract passed: Engine 运行边界已拆分，证据位于 $EVIDENCE"
