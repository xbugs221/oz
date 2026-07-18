# 简报

本次变更解决 `go-dag` 并行 subagent 在写入 `state.json` 时互相覆盖运行态的问题。用户在 `wo status/watch` 中会看到同一批 subagent 有些显示会话 ID，有些显示 `-`，即使这些 subagent 已经生成 member artifact，主流程也已经推进到 review、fix 或 qa。

交付目标：

- 并发 subagent 完成后，`state.json.sessions` 必须保留同一批所有 member 的会话 ID。
- subagent 完成时保存状态必须与最新 `state.json` 合并，不能用启动时的旧快照覆盖其他 member 的 sessions、DAG node 或阶段状态。
- subagent backend 输出 session started 事件后，运行态必须在 artifact 完成前就能持久化该 session，便于 `status/watch` 观察正在运行的 helper。
- `wo status/watch` 对已经完成的 parallel member 行必须显示对应 session ID，不再随机出现 `- ✓ -`。

非目标：

- 不恢复历史 run 中已经丢失的 session ID。
- 不改变 parallel group 的 member artifact 与 fan-in artifact 格式。
- 不改变 review/QA/fix/archive 的阶段推进规则。
- 不把 status 设计成从 agent 历史数据库反查 session；本次只修复 `state.json` 的写入一致性。

验收入口：

- `bash docs/changes/archive/2026-06-10-10-合并并行subagent会话状态并修正status观测/tests/test_parallel_subagent_session_state_contract.sh`

执行阶段默认上下文：

先读 `internal/app/go_dag.go`、`internal/app/subagent.go`、`internal/app/state.go`、`internal/app/status_view.go`。重点检查 `nodeRunSubagent`、`recordGoDAGNode`、`saveState/loadState` 和 `statusSubagentSessionID` 的协作关系。实现应优先新增小型状态合并 helper，而不是让并发节点继续保存整份旧 state。
