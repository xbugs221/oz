# 树状简化 wo 配置

本次变更把 `wo.yaml` 简化成和 `wo status` 一致的树状结构：根节点直接描述工作流，主阶段写在 `stages` 下，阶段前置子代理写在对应阶段的 `before` 中。用户不再需要理解 `wo.workflow`、`parallel.groups`、`cli/tool` 双字段、`permissions` 或轮次覆盖。

交付目标：

- 默认生成的新 `wo.yaml` 不再包含 `wo`、`workflow`、`engine`、`defaults`、`iterations`、`permissions`、`cli`、`tool`、`parallel.groups` 和 `mode`。
- 所有会话统一使用 `agent`，可选 `model`；未配置 `model` 时不传模型参数，继续使用 agent CLI 自身配置。
- `validation.max_attempts_per_stage` 改为 `validation.limit`。
- 顶层 `parallel: true/false` 控制所有阶段内 `before` 子代理是否启动。
- 默认子代理全部启动；每个子代理先判断当前提案是否与自身职责相关，无关时写出 `relevant: false` artifact 后立即退出。

非目标：

- 不兼容旧 `wo.yaml` 字段，也不提供自动迁移命令。
- 不开放 YAML 权限配置。
- 不校验模型名是否真实存在。
- 不让子代理直接修改源码或直接推进主流程。

验收入口：

- `bash docs/changes/24-树状简化wo配置/tests/test_tree_config_contract.sh`
- `bash docs/changes/24-树状简化wo配置/tests/test_legacy_config_rejection_contract.sh`
- `bash docs/changes/24-树状简化wo配置/tests/test_subagent_relevance_contract.sh`

执行阶段默认先读本简报、`acceptance.json` 和 `tests/`。`proposal.md`、`design.md` 和 `spec.md` 用于理解完整设计边界。
