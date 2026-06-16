#!/usr/bin/env bash
# 文件功能目的：验证工作流阶段状态和主阶段门禁流水线已经收敛到单一业务边界。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/38-stage-gate-pipeline"
LOG="$RESULT_DIR/contract.log"

mkdir -p "$RESULT_DIR"
: >"$LOG"

note() {
  printf '[stage-gate-pipeline] %s\n' "$*" | tee -a "$LOG"
}

fail() {
  printf '[stage-gate-pipeline] FAIL: %s\n' "$*" | tee -a "$LOG" >&2
  exit 1
}

assert_rg() {
  local pattern="$1"
  local path="$2"
  local message="$3"
  if ! rg -n "$pattern" "$path" >>"$LOG" 2>&1; then
    fail "$message"
  fi
}

note "检查阶段和状态语义边界"
assert_rg 'type workflowStage|type WorkflowStage' "$ROOT/internal/app" "缺少 workflow stage 语义类型"
assert_rg 'parseWorkflowStage|ParseWorkflowStage' "$ROOT/internal/app" "缺少 workflow stage 解析入口"
assert_rg 'type runStatus|type RunStatus|func normalizeRunStatus' "$ROOT/internal/app" "缺少 run status 规范化入口"

note "检查主阶段门禁流水线文件和结果结构"
pipeline_files="$(rg -l 'stageGatePipelineResult|completeMainStage|mainStageGatePipeline' "$ROOT/internal/app" || true)"
[[ -n "$pipeline_files" ]] || fail "缺少主阶段门禁流水线实现"
printf '%s\n' "$pipeline_files" >>"$LOG"
assert_rg 'stageGatePipelineResult|mainStageGatePipelineResult' "$ROOT/internal/app" "缺少门禁流水线结果结构"
assert_rg 'completeMainStage|runMainStageGatePipeline' "$ROOT/internal/app" "缺少主阶段门禁流水线入口"

note "检查 loop 和 DAG node 复用同一流水线"
assert_rg 'completeMainStage|runMainStageGatePipeline' "$ROOT/internal/app/engine_run.go" "runLoop 未调用主阶段门禁流水线"
assert_rg 'completeMainStage|runMainStageGatePipeline' "$ROOT/internal/app/node.go" "nodeRunStage 未调用主阶段门禁流水线"

node_body="$(awk '
  /func \(e \*Engine\) nodeRunStage/ { flag=1 }
  /func \(e \*Engine\) nodeStageDone/ { flag=0 }
  flag { print }
' "$ROOT/internal/app/node.go")"

if printf '%s\n' "$node_body" | rg -n 'runAcceptancePreflight|runAcceptanceGate|validateStage\(|markStageCompleted\(|advance\(' >>"$LOG" 2>&1; then
  fail "nodeRunStage 仍直接串联 acceptance、validation 或 advance，未收敛到流水线"
fi

note "运行 internal/app 回归测试"
(cd "$ROOT" && go test ./internal/app) >>"$LOG" 2>&1 || fail "go test ./internal/app 失败"

note "PASS: stage gate pipeline contract"
