# 简报

本次变更解决 `wo` 只能生成单一默认配置的问题。用户想快速试用多智能体对抗工作流，但当前需要手写 `wo.yaml` 中的并行角色、review/QA 门禁和验证配置，试用成本高，也容易把“插件系统”和“profile 预设”混在一起。

交付目标：

- 先把当前 `internal/app/config.go` 中类似 `mustDefaultWorkflowConfigYAML()` 的默认 `wo.yaml` 硬编码剥离为内置 YAML 模板文件，例如 `profiles-template/default.yaml`。
- MADA profile 也必须作为同类内置 YAML 模板维护，例如 `profiles-template/mada-code.yaml`、`profiles-template/mada-decision.yaml`、`profiles-template/mada-research.yaml`，让后续替换 subagent 描述、角色 purpose 和提示词方案时不必修改 Go 源码。
- `wo config --profile mada-code` 生成面向代码实现/审查的 MADA profile。
- `wo config --profile mada-decision` 生成面向技术选型、推荐和学习路线的 MADA decision profile。
- `wo config --profile mada-research` 生成面向资料调研、证据审计和结论交叉验证的 MADA research profile。
- `wo config --list-profiles` 能列出可用 profile、中文用途说明和推荐使用场景。
- `wo config --profile <name>` 仍写入标准 `wo.yaml`，不引入插件运行时或外部加载机制。
- 生成的 profile 必须能被 `wo graph --change <change> --format json` 正常加载，并展示对应并行角色。

非目标：

- 不实现动态 stage 插件系统。
- 不新增 Python/LangGraph 运行时。
- 不把 MADA 沙盒、Docker/E2B 或原位多人 Markdown 编辑纳入第一版。
- 不改变默认 `wo config` 的行为和默认 profile 内容。
- 不实现运行时从任意外部路径动态加载 profile；第一版只要求像 `prompts-template/*.md` 一样把内置模板从 Go 源码中剥离。
- 不改变 `codex`、`opencode`、`pi` 三类 agent backend 的注册方式。

验收入口：

- `bash docs/changes/11-新增-MADA-工作流profiles/tests/test_profile_templates_externalized_contract.sh`
- `bash docs/changes/11-新增-MADA-工作流profiles/tests/test_mada_profiles_config_contract.sh`
- `bash docs/changes/11-新增-MADA-工作流profiles/tests/test_mada_profile_discovery_contract.sh`

执行阶段默认上下文：

先读 `internal/app/config.go` 的 `mustDefaultWorkflowConfigYAML()`、默认 `parallel.groups` 硬编码和 prompt 嵌入逻辑，再读 `prompts-template/embed.go` 的内置模板模式、`internal/app/app.go` 的 `wo config` 参数解析路径、`internal/app/graph.go` 的并行组图展开逻辑。实现顺序必须先抽离默认配置模板，再在同一模板机制上增加 MADA profiles，避免新增插件抽象。
