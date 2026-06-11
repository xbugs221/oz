# 规格

## 验收矩阵

| 需求 | 场景 | 测试 | 证据 | 风险 |
| --- | --- | --- | --- | --- |
| 已完成执行跳过上下文 subagents | task 全部完成时不启动执行前 subagents | `skip-execution-context-when-tasks-done` | `skip-execution-context-log`, `skip-execution-context-state` | 只覆盖 execution 前 advisory groups，不覆盖 review/QA gate input |
| 未完成执行保留上下文 subagents | task 未完成时仍启动执行前 subagents | `run-execution-context-when-tasks-pending` | `pending-execution-context-log`, `pending-execution-context-state` | fake agent 只模拟最小执行副作用，不评价真实模型输出质量 |

### 需求：已完成执行跳过上下文 subagents

系统必须在 execution 前先判断当前 oz change 的 task 是否已经全部完成；如果已经完成，不得再启动服务于 execution 的代码侦察和外部资料 advisory subagents。

#### 场景：task 全部完成时不启动执行前 subagents

- **给定** 一个 active change 的 `task.md` 已全部勾选
- **并且** `wo.yaml` 启用了 `implementation_context` 中的代码侦察和外部资料 advisory subagents
- **当** 用户运行 `wo run --change <change> --json`
- **则** workflow 不得调用这些 execution 前 subagents
- **并且**不得生成 execution context member artifact 或 subagent session
- **并且** workflow 仍可继续进入 archive 并完成
- **测试**：`docs/changes/17-已完成执行跳过上下文subagents/tests/test_skip_execution_context_when_tasks_done.sh`
- **真实数据来源**：脚本创建临时 git 仓库、真实 oz change 文件、真实 `wo.yaml` 并运行当前仓库构建出的 `wo` 和 `oz`
- **入口路径**：`cmd/wo run --change <change> --json`
- **关键断言**：fake subagent 一旦收到 `SUBAGENT_OUTPUT` 就让测试失败；最终 state 必须为 `done` 且无 subagent session/artifact
- **剩余风险**：该测试不覆盖 review/QA subagents，因为本次只收窄 execution 前置上下文

### 需求：未完成执行保留上下文 subagents

系统必须在 task 未完成时保留既有 execution 前 advisory subagents，避免为了节约已完成路径而破坏正常执行路径。

#### 场景：task 未完成时仍启动执行前 subagents

- **给定** 一个 active change 的 `task.md` 尚未勾选
- **并且** `wo.yaml` 启用了 `implementation_context` 中的代码侦察和外部资料 advisory subagents
- **当** 用户运行 `wo run --change <change> --json`
- **则** workflow 必须先调用这些 execution 前 subagents 并生成 fan-in artifact
- **并且** execution 主 agent 可以继续勾选 task
- **并且** workflow 可进入 archive 并完成
- **测试**：`docs/changes/17-已完成执行跳过上下文subagents/tests/test_run_execution_context_when_tasks_pending.sh`
- **真实数据来源**：脚本创建临时 git 仓库、真实 oz change 文件、真实 `wo.yaml`，fake subagents 写入真实 member artifact
- **入口路径**：`cmd/wo run --change <change> --json`
- **关键断言**：两个 configured subagents 都被调用；`parallel-implementation-context.json` 包含两个成员；最终 state 为 `done`
- **剩余风险**：fake execution 只勾选 task，不代表真实 agent 修改业务代码；真实修改质量仍由具体提案测试和 QA 负责

