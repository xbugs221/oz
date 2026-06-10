#!/usr/bin/env bash
# 文件功能目的：验证 go-dag 并发 subagent 保存 state.json 时不会丢失 sibling session/DAG node，并且 running session 能提前观测。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
result_dir="$repo_root/test-results/10-parallel-subagent-session-state"
log="$result_dir/parallel-subagent-session-state.log"
regression_log="$result_dir/status-subagent-godag-regression.log"

mkdir -p "$result_dir"
: >"$log"
: >"$regression_log"

note() {
  # note 记录合同执行步骤，方便执行阶段定位失败点是业务行为还是测试环境。
  printf '%s\n' "$*" | tee -a "$log"
}

cd "$repo_root"

note "运行 internal/app 持久化合同测试，复现并发 subagent state 合并问题"
note "初始失败点摘要：旧实现会用 subagent 启动时的旧 state 快照整份保存，导致 sibling sessions 互相覆盖"
note "初始失败点摘要：旧实现不会在 helper artifact 完成前把 running session started 事件持久化到 state.json"
note "初始失败点摘要：旧实现的 DAGNodes 记录若使用 load-modify-save，可能在 sibling node 并发写入时丢失状态"
note "初始失败点摘要：旧实现的 run_id 读写权威不一致，污染 state.json 后可能影响后续 artifact 路径"

if ! go test -race ./internal/app -run 'TestParallelSubagentSessionsAreMergedWithoutDroppingPeers|TestRunningSubagentSessionIsPersistedBeforeArtifactCompletion|TestGoDAGNodeStatesAreMergedWithoutDroppingPeers|TestMergeStateRejectsMismatchedRunID|TestLoadStateRejectsMismatchedRunID' -count=1 2>&1 | tee -a "$log"; then
  note "合同测试失败；如果失败点是缺少 subagent session，则符合创建阶段预期"
  exit 1
fi

note "运行 task 4.2 现有回归测试并保存独立日志：$regression_log"
if ! go test ./internal/app -run 'Status|Subagent|GoDAG' -count=1 2>&1 | tee "$regression_log" | tee -a "$log"; then
  note "task 4.2 回归测试失败；详见 $regression_log"
  exit 1
fi

note "PASS"
note "通过断言摘要：final-state.json 保留 executor session 和全部 implementation_context subagent session"
note "通过断言摘要：running-state.json 在 artifact 完成前包含 running subagent session，status subagent row 可观测且父阶段 progress 未被 helper session 污染"
note "通过断言摘要：dag-node-state.json 保留 barrier 压力下 sibling DAGNodes 的 Status、StartedAt、FinishedAt、Artifact、Error"
note "通过断言摘要：mergeState/loadState 均拒绝污染的 run_id，未创建 run 目录外 artifact"
note "通过断言摘要：task 4.2 回归命令输出已保存到 $regression_log"
