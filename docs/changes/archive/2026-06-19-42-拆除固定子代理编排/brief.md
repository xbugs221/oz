# 简报：拆除固定子代理编排

当前 `oz flow` 在 execution、review、QA 等主阶段前预先调度固定的外置子代理，并把这些 helper 的 artifact、session 和 fan-in 状态纳入 workflow 观测。这个设计和 Codex/Pi 命令行自身的原生 sub agent 能力重复，也让 oz 在阶段主代理之前过早替它判断需要哪些信息。

本次变更目标是在不改变主流程阶段的前提下，拆除 oz 自己的固定外置子代理编排。`oz flow` 继续负责 execution、review、QA、fix、archive 的状态机、产物门禁、acceptance 和 validation；阶段内部是否调用 sub agent、调用哪些 sub agent、如何聚焦任务，完全交给当前阶段主代理及其 backend 配置。

交付目标：

- 默认 `oz-flow.yaml` 不再生成 `parallel`、`subagent_guard` 或 `stages.<stage>.before` 固定子代理配置。
- 默认 workflow graph 只包含主阶段和 gate，不再包含 subagent、fan-in 或 parallel artifact 节点。
- execution/review/QA prompt 不再要求主代理读取 oz 生成的 subagent/parallel artifact。
- `oz flow status/watch` 不再观测或展示外置子代理会话，只展示主阶段状态和主阶段 agent session。
- 旧外置子代理配置字段明确报错，避免静默忽略造成误解。

非目标：

- 不改变 execution、review、QA、fix、archive 的主流程顺序。
- 不改变 review/QA/fix/archive artifact schema。
- 不要求 oz 理解 Codex/Pi 内部 sub agent 的会话、日志或产物。
- 不为历史运行态继续维护外置子代理调度与 fan-in 兼容路径。

验收入口：

- `bash docs/changes/archive/2026-06-19-42-拆除固定子代理编排/tests/test_remove_fixed_subagents_contract.sh`
- `oz validate 42-拆除固定子代理编排 --json`

执行阶段默认上下文：先运行创建阶段契约测试，确认失败点来自现有固定子代理编排仍存在；再按 `task.md` 拆除配置、DAG、prompt 和 status 观测边界。实现时不要新增替代性的 oz 子代理调度层。
