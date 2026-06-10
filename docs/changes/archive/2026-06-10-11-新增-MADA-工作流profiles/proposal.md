# 提案

## 问题

`wo` 已经具备 Go DAG、并行 subagent、fan-in artifact、review/QA gate 和 fix loop，但用户要试用 MADA 风格的多智能体对抗工作流时，仍需要手写复杂 `wo.yaml`。这会带来三个问题：

- 默认 `wo.yaml` 中的 subagent 描述、角色 purpose 和 prompts 配置仍由 Go 字符串拼接维护，调整一套提示词方案必须改源码。
- 试用者先被配置细节拦住，无法快速验证 MADA 是否真的提高方案质量。
- 不同用户会随意命名角色，导致 review/QA artifact 的语义不稳定。
- 容易误以为要先做插件系统，实际第一版只需要一组可审计的配置预设。

## 目标

先把默认工作流配置从 `internal/app/config.go` 的硬编码中迁移到类似 `prompts-template/*.md` 的内置 YAML 模板目录，例如 `profiles-template/default.yaml`。默认 `wo config` 行为和输出语义保持不变，但后续替换 subagent 描述、角色 purpose 和 prompts 方案时不再需要修改 Go 源码。

再新增一组同样由 `profiles-template/*.yaml` 维护的内置 profile，让用户可以用 `wo config --profile <name>` 直接生成可运行的 `wo.yaml`：

- `mada-code`：面向代码实现、审查、QA 和修复循环。
- `mada-decision`：面向技术选型、框架推荐、学习路线和决策报告。
- `mada-research`：面向资料调研、外部证据审计和结论交叉验证。

同时新增 `wo config --list-profiles`，让用户看到可用 profile 及其用途。

## 为什么现在做

当前 MADA 还处在快速验证阶段。最小可行路径不是从头实现 Python/LangGraph，也不是设计插件系统，而是复用 `wo` 已有状态机和并行 artifact 合同，用 profile 降低试用成本。只要 profile 能生成标准 `wo.yaml` 并通过现有 `wo graph`、`wo run` 路径加载，就能先验证多角色对抗流程是否有效。

先抽离默认模板可以把“配置内容维护”和“配置读取逻辑”分开，避免为了新增或替换一组角色方案反复改 Go 源码。

## 非目标

- 不新增 `wo plugin` 命令。
- 不允许外部 Go plugin、脚本或远程模板注入 profile。
- 不支持运行时从任意本地路径加载 profile；第一版只支持随二进制内置的 YAML 模板。
- 不改变默认 `wo config` 行为。
- 不把 Docker sandbox 作为内置核心能力。
- 不保证 profile 覆盖所有行业场景；第一版只服务个人开发者和小团队的本地试用。
