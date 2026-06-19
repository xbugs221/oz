# 提案：拆除固定子代理编排

## 背景

`oz flow` 最近引入了阶段前并行子代理：execution 前跑代码侦察和外部资料，review 前跑目标核对、测试有效性、安全风险和上下文一致性，QA 前跑 CLI/API、浏览器路径和回归场景。这套能力由 oz 自己生成 DAG 节点、调用 agent backend、收集 member artifact，再把 fan-in artifact 注入主阶段 prompt。

问题是 Codex 和 Pi 命令行本身已经有原生 sub agent 配置和提示词。oz 再做一层固定子代理编排，会把“阶段内是否需要委派”这个动态判断提前固化在 workflow 配置里。对简单 CLI 变更、纯文档变更或高度局部的修复来说，这会产生无关 helper、额外等待和额外状态噪音。

## 变更目标

本次变更保留 `oz flow` 的主流程，但删除 oz 外置固定子代理层：

- 主流程仍是 execution -> review -> QA -> fix/archive。
- 阶段主代理仍由 `stages.<stage>.agent` 和 reasoning 配置驱动。
- 阶段产物仍由 `task.md`、`review-N.json`、`qa-N.json`、`fix-N-summary.md` 和 `delivery-summary.md` 证明。
- 阶段内部是否使用 sub agent 完全交给 Codex/Pi backend 自身，不由 oz 配置或观测。

## 用户可见行为

用户生成默认配置时，不再看到 `parallel`、`subagent_guard` 和 `before` 固定成员列表。

用户查看 graph/status/watch 时，不再看到 subagent、fan-in、parallel artifact 或 helper session。状态视图只表达主阶段和 gate 进度。

用户保留旧配置时，`oz flow` 应明确报错说明这些字段已删除，而不是静默忽略。

## 非目标

不把 Codex/Pi 内部 sub agent 的观测协议接入 oz。

不引入新的 helper policy、helper budget 或动态 delegation artifact。

不改变 review/QA 的 clean/needs_fix 判定规则，也不弱化 acceptance/validation gate。
