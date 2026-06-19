# 设计：拆除固定子代理编排

## 决策

`oz flow` 的职责收敛为外层工作流控制，不再拥有阶段前外置子代理调度。阶段内部信息搜集和任务拆分属于当前 agent backend 的能力边界。

这意味着需要删除或停用以下 oz 责任：

- 从 `stages.<stage>.before` 解析固定 helper。
- 在 workflow graph 中生成 `subagent` 和 `fanin` 节点。
- 调用 `nodeRunSubagent`、member artifact retry、parallel fan-in 和 helper-only read-only guard。
- 在 prompt context 中注入 `ParallelContextPath`、`ParallelReviewPath`、`ParallelQAPath` 等路径。
- 在 status/watch 中展示 subagent session 或 fan-in artifact。

保留以下 oz 责任：

- 主阶段状态机和 stage decision。
- 主阶段 prompt 渲染、agent session 续跑和 artifact gate。
- acceptance preflight、acceptance run 和 validation commands。
- review/QA/fix/archive 的产物 schema 和重试门禁。

## 配置迁移

新默认配置不生成外置子代理字段。旧字段必须明确拒绝：

- `parallel`
- `subagent_guard`
- `subagents`
- `stages.<stage>.before`

选择拒绝而不是静默忽略，是为了避免用户认为固定子代理仍会被调度。错误信息应包含字段名，并说明该字段已删除或不再支持。

## Prompt 收敛

execution prompt 只要求读取 `state.json`、当前 change 和 acceptance。review/QA prompt 只读取主阶段必要输入，例如当前 change、acceptance、review/QA artifact 路径和历史轮次 artifact。prompt 不再提及 subagent artifact、parallel artifact、review helper 或 QA helper。

Codex/Pi 是否在阶段内部派发 sub agent，由它们自己的配置和提示词决定。oz 不添加额外提示，也不要求它们回传内部委派细节。

## 状态与图

默认 graph 只表达主阶段和 gate。status/watch 只显示主阶段状态、主阶段 agent session、run path 和错误。历史 run 如果残留旧 subagent DAG 节点，新的公共状态视图也不应把它们展示为当前观测面。

## 风险

删除外置 helper 后，oz 不再单独重试 helper artifact，也无法对 helper 运行做独立只读边界保护。这个风险由主阶段 artifact gate、manual intervention 检测和 backend 自身能力承担。

旧运行态可能包含已经生成的 subagent artifact。执行阶段可以选择只保证新 run 行为；历史运行态的清理由 `oz flow clean` 或手动清理处理。
