# 规格

## 验收矩阵

| 需求 | 场景 | 测试 | 证据 | 风险 |
| --- | --- | --- | --- | --- |
| 并行 subagent 会话状态不互相覆盖 | implementation_context 多成员并发完成后保留全部 session | `parallel-subagent-session-state` | `parallel-subagent-session-log`, `parallel-subagent-final-state` | 不恢复历史 run，只验证新 run 的写入一致性 |
| status 展示完成 subagent 的真实 session | 已完成 member artifact 对应的 status 行显示 session ID | `parallel-subagent-session-state` | `parallel-subagent-session-log`, `parallel-subagent-final-state` | status 仍依赖 state.json，不从外部 agent 数据库反查 |
| 运行中 subagent session 可提前观测 | backend 输出 session started 后，artifact 完成前 state 已包含 session | `parallel-subagent-session-state` | `parallel-subagent-session-log`, `parallel-subagent-running-state` | 只验证 session 提前持久化，不要求未完成 member 有成功 marker |

### 需求：并行 subagent 会话状态不互相覆盖

系统必须让同一 parallel group 中并发运行的 subagent 只合并自己的状态增量，不得用启动时的旧 `state.json` 快照覆盖其他 subagent 已写入的 session。

#### 场景：implementation_context 多成员并发完成后保留全部 session

- **给定** 一个 running `execution` run，`implementation_context` 配置了多个 `pi` subagent member
- **当** 这些 member 使用各自的旧 state 快照并发完成，并分别写出 member artifact 和 session ID
- **则** 最终 `state.json.sessions` 必须包含每个 member 的 session key
- **并且** 不得丢失主阶段已有 session
- **测试**：`docs/changes/archive/2026-06-10-10-合并并行subagent会话状态并修正status观测/tests/test_parallel_subagent_session_state_contract.sh`
- **真实数据来源**：测试在临时 git 仓库中创建真实 run state，调用 `nodeRunSubagent` 写真实 `parallel-members/implementation_context/*.json`
- **入口路径**：`nodeRunSubagent`、`saveState/loadState` 或新增状态合并 helper
- **关键断言**：全部 subagent session key 都存在；每个 member artifact 都存在；最终 state snapshot 写入 `test-results`
- **剩余风险**：该场景不修复历史 run 中已经丢失的 session

### 需求：status 展示完成 subagent 的真实 session

系统必须让 `wo status/watch` 的 subagent 行反映 durable state 中已经记录的 session ID。已完成 member artifact 对应的行不应因为并发保存覆盖而随机显示 `-`。

#### 场景：已完成 member artifact 对应的 status 行显示 session ID

- **给定** 并发 subagent 已完成，member artifact 存在，`state.json.sessions` 应包含每个 member 的 session
- **当** 系统构建 compact status view
- **则** 每个完成 member 行必须显示对应 session ID
- **并且** marker 必须仍是完成态 `✓`
- **测试**：`docs/changes/archive/2026-06-10-10-合并并行subagent会话状态并修正status观测/tests/test_parallel_subagent_session_state_contract.sh`
- **真实数据来源**：同一测试中由 fake runner 通过真实 subagent prompt 写出的 member artifact 和真实 `state.json`
- **入口路径**：`buildStatusView`、`statusSubagentRows`、`statusSubagentSessionID`
- **关键断言**：每个 member 的 row `SessionID` 等于对应 session；row `Marker` 为 `✓`
- **剩余风险**：status 不从外部 agent 历史记录补救缺失 session，缺失状态应由写入路径修复

### 需求：运行中 subagent session 可提前观测

系统必须在 subagent backend 输出 session started 事件后立即把 session 合并进 `state.json`，不能等到 member artifact 完成后才保存。

#### 场景：backend 输出 session started 后，artifact 完成前 state 已包含 session

- **给定** 一个 subagent runner 在写 artifact 前先输出 `agent session started`
- **当** subagent 仍阻塞在运行中，member artifact 尚未完成
- **则** `state.json.sessions` 必须已经包含该 subagent 的 session key
- **测试**：`docs/changes/archive/2026-06-10-10-合并并行subagent会话状态并修正status观测/tests/test_parallel_subagent_session_state_contract.sh`
- **真实数据来源**：测试 runner 实现 progress writer，在 artifact 写入前发送 session started 事件并阻塞
- **入口路径**：`nodeRunSubagent` 与 agent runner progress writer 的连接点
- **关键断言**：释放 runner 前读取 `state.json`，能看到对应 subagent session；释放后 goroutine 正常完成
- **剩余风险**：该场景不要求未完成 member 行显示成功，只要求 session 可观测
