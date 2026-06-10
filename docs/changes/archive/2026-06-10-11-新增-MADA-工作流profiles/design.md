# 设计

## 决策

采用“内置 YAML profile 模板”而不是插件系统。第一步先把当前 `mustDefaultWorkflowConfigYAML()` 里的默认 `wo.yaml` 字符串拼接迁移到模板文件，后续 MADA profile 复用同一机制。

```text
wo config
├── 默认：profiles-template/default.yaml
├── --profile mada-code：profiles-template/mada-code.yaml
├── --profile mada-decision：profiles-template/mada-decision.yaml
├── --profile mada-research：profiles-template/mada-research.yaml
└── --list-profiles
```

profile 的输出仍是普通 `wo.yaml`：

```text
profiles-template/*.yaml
  -> go:embed
  -> wo.yaml
  -> LoadWorkflowConfig
  -> BuildWorkflowSpec
  -> go-dag / status / review / qa 现有路径
```

这样可以复用现有配置解析、状态快照、parallel gate 和 graph 导出能力，不需要新增运行时协议。

## 模板组织

新增一个类似 `prompts-template/` 的目录，例如：

```text
profiles-template/
├── embed.go
├── default.yaml
├── mada-code.yaml
├── mada-decision.yaml
└── mada-research.yaml
```

- `default.yaml` 保存现有默认 `wo config` 的标准 YAML 输出语义，包括默认阶段参数、parallel groups、subagent 角色名、purpose 和 prompt 配置入口。
- 三个 MADA YAML 文件保存各自 profile 的完整标准 `wo.yaml` 内容。
- `internal/app/config.go` 只保留读取、校验、列出和写入模板的逻辑，不再用 Go 字符串数组硬编码默认 subagent 名称和 purpose。
- `prompts-template/*.md` 继续保存阶段主提示词；profile YAML 可以直接包含 prompts 内容，也可以通过实现层复用现有 `defaultPromptSet()` 注入，但 subagent 描述和 profile 结构必须在 YAML 模板文件中维护。

## Profile 语义

### mada-code

用于代码实现和审查场景。角色重点：

- 规划上下文：需求分析员、代码库侦察员、风险分析员。
- 执行上下文：架构约束侦察员、测试入口侦察员。
- Review：Planner、Skeptic、Maintainer、Evidence Checker。
- QA：Contract Runner、Evidence Auditor。

### mada-decision

用于类似“推荐一个最适合个人开发者/小团队的全栈 app 开发框架和快速学习方案”的决策问题。角色重点：

- 规划上下文：需求澄清员、约束建模员、候选池研究员。
- Review：候选方案研究员、反方评审员、运维部署评审员、学习路线评审员、证据审计员。
- QA：决策矩阵审计员、学习计划审计员。

### mada-research

用于资料调研、外部文档核验和结论交叉验证。角色重点：

- 规划上下文：问题分解员、资料范围规划员、证据标准制定员。
- Review：一手资料研究员、反例搜索员、证据审计员、结论压缩员。
- QA：引用和证据审计员、复现实验审计员。

## 命令行为

```text
wo config --profile mada-decision
```

- 在当前 git 仓库写入 `wo.yaml`。
- 如果 `wo.yaml` 已存在，沿用现有“不覆盖”错误。
- 生成的 YAML 必须能被 `LoadWorkflowConfig` 读取。

```text
wo config --global --profile mada-code
```

- 写入 `~/wo.yaml`。
- 保持现有全局配置位置，不创建 `~/.wo`。

```text
wo config --list-profiles
```

- 输出 `default`、`mada-code`、`mada-decision`、`mada-research`。
- 每个 profile 至少包含一行中文用途说明。

未知 profile 必须返回非零错误，并提示可用 profile 名称。

## 取舍

- 内置 profile 可维护、可测试，适合第一版验证。
- 抽离到 YAML 模板后，替换内置提示词方案只需要改模板文件并重新构建，不需要改 Go 源码。
- 不做插件系统，避免把问题扩大为扩展框架设计。
- 不支持运行时从任意外部目录加载 profile，避免引入信任边界、路径解析和配置兼容性问题。
- 不新增动态 stage，避免破坏 `wo` 当前有限 DAG 和 artifact gate 语义。

## 风险

- profile 名称和角色命名可能过早固化。缓解：第一版只承诺三个试用 profile，后续可新增 profile，但不承诺兼容外部插件。
- `mada-decision` 属于非编码工作流，可能与 `execution/review/qa/fix` 命名有语义偏差。缓解：第一版只要求配置和图可运行，具体报告产物由提案 `acceptance.json` 约束。
- 生成 YAML 内容较长。缓解：长内容放在 `profiles-template/*.yaml`，Go 代码只处理模板读取和校验。
