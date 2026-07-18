# 任务

## 1. 契约测试

- [x] 1.1 运行 `bash docs/changes/archive/2026-06-10-10-合并并行subagent会话状态并修正status观测/tests/test_parallel_subagent_session_state_contract.sh`，确认当前实现失败于并发 subagent session 丢失或 running session 未提前持久化，而不是测试语法错误
- [x] 1.2 保留测试输出到 `test-results/10-parallel-subagent-session-state/parallel-subagent-session-state.log`

## 2. 状态合并实现

- [x] 2.1 新增小型状态合并 helper，在同一锁内读取最新 `state.json`、应用增量、写回
- [x] 2.2 将 `recordGoDAGNode` 改为只合并当前 `DAGNodes[nodeID]`，避免覆盖其他节点状态
- [x] 2.3 将 `nodeRunSubagent` 完成后的 session 保存改为合并当前 subagent session key，避免覆盖其他 sessions
- [x] 2.4 为 subagent runner 接入 progress writer，在 session started 事件出现时立即合并当前 subagent session

## 3. status 观测

- [x] 3.1 确认 `statusSubagentSessionID` 的 key 规则与 `nodeRunSubagent` 写入规则一致
- [x] 3.2 确认完成 member artifact 对应的 status 行显示 session ID 和 `✓`
- [x] 3.3 不通过从 artifact 猜测 session 或从外部 agent 数据库反查 session 来掩盖写入问题

## 4. 回归验证

- [x] 4.1 重新运行 `bash docs/changes/archive/2026-06-10-10-合并并行subagent会话状态并修正status观测/tests/test_parallel_subagent_session_state_contract.sh`
- [x] 4.2 运行与 status/watch 相关的现有回归测试，至少覆盖 `go test ./internal/app -run 'Status|Subagent|GoDAG' -count=1`
- [x] 4.3 更新 `task.md` 勾选状态，并在执行总结中列出 final/running state snapshot 路径
