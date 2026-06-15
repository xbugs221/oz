#!/usr/bin/env bash
# Sources: 10-合并并行subagent会话状态并修正status观测
# 文件功能目的：验证 内嵌工作流 并发 subagent 保存 state.json 时不会丢失 sibling session/DAG node，并且 running session 能提前观测。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
result_dir="$repo_root/test-results/session-merge-contract"
log="$result_dir/parallel-subagent-session-merge.log"
regression_log="$result_dir/status-subagent-godag-regression.log"

mkdir -p "$result_dir"
: >"$log"
: >"$regression_log"

note() {
  printf '%s\n' "$*" | tee -a "$log"
}

cd "$repo_root"

note "运行 internal/app 持久化合同测试：并发 subagent session 合并、running session 提前持久化、DAG node 合并、run_id 污染拒绝"
if ! go test -race ./internal/app -run 'TestParallelSubagentSessionsAreMergedWithoutDroppingPeers|TestRunningSubagentSessionIsPersistedBeforeArtifactCompletion|TestGoDAGNodeStatesAreMergedWithoutDroppingPeers|TestMergeStateRejectsMismatchedRunID|TestLoadStateRejectsMismatchedRunID' -count=1 2>&1 | tee -a "$log"; then
  note "合同测试失败"
  exit 1
fi

note "运行 Status/Subagent/GoDAG 回归测试并保存独立日志：$regression_log"
if ! go test ./internal/app -run 'Status|Subagent|GoDAG' -count=1 2>&1 | tee "$regression_log" | tee -a "$log"; then
  note "回归测试失败；详见 $regression_log"
  exit 1
fi

note "PASS"
note "通过断言摘要：final-state.json 保留 executor session 和全部 implementation_context subagent session"
note "通过断言摘要：running-state.json 在 artifact 完成前包含 running subagent session"
note "通过断言摘要：dag-node-state.json 保留 barrier 压力下 sibling DAGNodes 的完整状态"
note "通过断言摘要：mergeState/loadState 均拒绝污染的 run_id"
note "通过断言摘要：回归命令输出已保存到 $regression_log"
