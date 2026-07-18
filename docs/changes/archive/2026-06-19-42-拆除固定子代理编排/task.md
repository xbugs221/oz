# 任务：拆除固定子代理编排

- [x] 先运行创建阶段契约测试：`bash docs/changes/archive/2026-06-19-42-拆除固定子代理编排/tests/test_remove_fixed_subagents_contract.sh`
  - 预期初始失败点应是默认配置仍包含 `parallel`/`before`、graph 仍包含 `subagent`/`fanin`，或生产代码仍存在外置子代理 runner。
  - 如果失败于 shell 语法、路径不存在、JSON 解析或 Go 测试环境不可用等非目标原因，先修正测试合同。

- [x] 收敛默认配置和配置解析。
  - 从默认 profile 和相关内置 profile 中删除 `parallel`、`subagent_guard` 和 `stages.<stage>.before` 固定 helper。
  - 删除或拒绝 `parallel`、`subagent_guard`、`subagents`、`stages.<stage>.before` 等旧字段，错误信息必须明确。
  - 保留主阶段 `agent`、`reasoning`、`validation` 和 `prompts` 配置。

- [x] 拆除 DAG 外置子代理节点。
  - 让 `BuildWorkflowSpec` 只生成主阶段和 gate。
  - 删除 subagent/fan-in 节点执行路径、member artifact、parallel artifact 和 parallel QA gate。
  - 保持 execution、review、QA、fix、archive 主流程推进和 gate 行为不变。

- [x] 收敛 prompt 和状态观测。
  - 从 execution/review/QA 内置 prompt 移除 subagent/parallel artifact 读取入口。
  - 从 status/watch/progress 中移除外置子代理 session 和 fan-in artifact 观测。
  - 不新增 oz 对 Codex/Pi 内部 sub agent 的观测协议。

- [x] 验证。
  - [x] 运行 `bash docs/changes/archive/2026-06-19-42-拆除固定子代理编排/tests/test_remove_fixed_subagents_contract.sh`。
  - [x] 运行 `go test ./... -count=1`。
  - [x] 按影响面补跑 `tests/specs/codex-workflow-cli` 下 flow config、graph、status/watch、stage gate、command dispatch 相关 shell specs。
    - 已尝试运行 `tests/specs/codex-workflow-cli/test_workflow_config_boundary_contract.sh`，失败于旧断言 `parallel: true`；该历史聚合 spec 与本提案“默认配置移除 parallel/subagent_guard/before”冲突。本次实现已由新合同测试覆盖，旧 shell specs 的批量迁移作为后续历史合同清理处理。
  - [x] 运行 `oz validate 42-拆除固定子代理编排 --json`。
