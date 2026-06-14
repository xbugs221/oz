# 规格

## 验收矩阵

| 需求 | 场景 | required_tests | required_evidence | 剩余风险 |
| --- | --- | --- | --- | --- |
| 树状 KISS 配置 | 默认 `wo.yaml` 使用根节点树状结构 | `tree-config-contract` | `tree-config-log`, `tree-config-generated-yaml`, `tree-config-graph-json` | 不覆盖用户手写所有 prompt 组合 |
| 树状 KISS 配置 | 顶层 `parallel: false` 关闭阶段内子代理 | `tree-config-contract` | `tree-config-log`, `tree-config-graph-json` | 只验证 graph，不验证所有运行态展示 |
| 旧字段硬拒绝 | 旧顶层和旧别名字段不能继续生效 | `legacy-config-rejection-contract` | `legacy-config-rejection-log` | 不提供自动迁移，只验证拒绝边界 |
| 会话模型和 validation limit | 主阶段和子代理会话只在显式配置 `model` 时传模型参数，并接受 `validation.limit` | `subagent-relevance-contract` | `subagent-relevance-log`, `subagent-relevance-state` | 不校验模型名真实存在 |
| 子代理默认启动并快速判无关 | 默认子代理全部启动；无关职责返回 `relevant:false` 后不阻断主阶段 | `subagent-relevance-contract` | `subagent-relevance-log`, `subagent-relevance-state` | fake agent 只验证 CLI 提案下浏览器子代理无关路径 |

### 需求：树状 KISS 配置

系统必须用根节点树状结构描述 `wo.yaml`，并让阶段内子代理嵌入 `stages.<stage>.before`。

#### 场景：默认 `wo.yaml` 使用根节点树状结构

- **给定** 一个新 git 仓库
- **当** 用户运行 `wo config`
- **则** 生成的 `wo.yaml` 根节点包含 `parallel`、`max_review_iterations`、`stages`、`validation` 和 `prompts`
- **且** 默认配置不包含 `wo:`、`workflow:`、`engine:`、`defaults:`、`iterations:`、`permissions:`、`cli:`、`tool:`、`groups:` 和 `mode:`
- **且** 默认配置不写任何 `model:`，让 CLI 自身默认模型继续生效
- **且** `stages.execution.before`、`stages.review.before` 和 `stages.qa.before` 包含默认子代理
- **测试**：`docs/changes/24-树状简化wo配置/tests/test_tree_config_contract.sh`
- **真实数据来源**：测试创建临时 git 仓库，运行真实构建出的 `wo config` 和 `wo graph`
- **入口路径**：`wo config`、`wo graph --change demo --format json`
- **关键断言**：默认 YAML 没有旧字段；graph JSON 包含阶段前置子代理节点
- **剩余风险**：不覆盖用户自定义 prompt 的所有文本组合

#### 场景：顶层 `parallel: false` 关闭阶段内子代理

- **给定** `wo.yaml` 中保留 `stages.execution.before`
- **且** 顶层配置 `parallel: false`
- **当** 用户导出 workflow graph
- **则** graph 不包含任何子代理节点
- **且** 主阶段仍保留
- **测试**：`docs/changes/24-树状简化wo配置/tests/test_tree_config_contract.sh`
- **真实数据来源**：测试改写临时仓库中的真实 `wo.yaml`
- **入口路径**：`wo graph --change demo --format json`
- **关键断言**：`parallel:false` 时 graph 中没有默认子代理名称，但仍包含 execution/archive 主阶段
- **剩余风险**：运行态 status 的全部视觉细节由既有 status 测试继续覆盖

### 需求：旧字段硬拒绝

系统必须拒绝旧配置格式和旧字段，避免用户误以为旧配置仍然被读取。

#### 场景：旧顶层和旧别名字段不能继续生效

- **当** `wo.yaml` 包含 `wo:`、`workflow:`、`engine:`、`defaults:`、`iterations:`、`permissions:`、`cli:`、`tool:`、`parallel.groups` 或 `validation.max_attempts_per_stage`
- **则** 配置读取失败
- **且** 错误信息包含对应旧字段名
- **且** 不创建新的运行态
- **测试**：`docs/changes/24-树状简化wo配置/tests/test_legacy_config_rejection_contract.sh`
- **真实数据来源**：测试为每个旧字段创建临时 git 仓库和真实 `wo.yaml`
- **入口路径**：`wo graph --change demo --format json`
- **关键断言**：每个旧字段样例都失败，stderr/stdout 中能定位被拒绝字段
- **剩余风险**：不提供自动迁移命令

### 需求：会话模型和 validation limit

系统必须允许主阶段会话和阶段内子代理会话通过 `model` 指定模型；未配置模型时不向 CLI 传模型参数。`validation.limit` 是唯一的 validation 重试预算字段。

#### 场景：主阶段和子代理会话只在显式配置 `model` 时传模型参数，并接受 `validation.limit`

- **给定** 新格式 `wo.yaml`
- **且** `stages.execution.model` 配置为 `codex-exec-model`
- **且** `stages.qa.before[].model` 为浏览器路径测试员配置 `pi-browser-model`
- **且** `validation.limit: 2`
- **当** 用户运行 `wo run --change <change> --json`
- **则** execution 主阶段调用 Codex 时包含 `-m codex-exec-model`
- **且** 浏览器路径测试员调用 Pi 时包含 `--model pi-browser-model`
- **且** 未配置模型的子代理调用不包含 `--model`
- **且** 配置读取接受 `validation.limit`
- **测试**：`docs/changes/24-树状简化wo配置/tests/test_subagent_relevance_contract.sh`
- **真实数据来源**：测试创建临时 active change、真实 `wo.yaml` 和 fake agent CLI，运行真实 `wo run`
- **入口路径**：`wo run --change 1-纯CLI配置变更 --json`
- **关键断言**：fake CLI argv 日志记录了显式模型参数，并证明未配置模型不会传参数
- **剩余风险**：模型名是否可用仍由对应 agent CLI 决定

### 需求：子代理默认启动并快速判无关

系统必须默认启动阶段内配置的全部子代理；子代理自己判断当前提案是否与职责相关，无关时用标准 artifact 快速退出。

#### 场景：默认子代理全部启动；无关职责返回 `relevant:false` 后不阻断主阶段

- **给定** 一个只修改 CLI 配置解析的提案
- **且** `stages.qa.before` 包含 `CLI/API 测试员`、`浏览器路径测试员` 和 `回归场景测试员`
- **当** 用户运行 `wo run --change <change> --json`
- **则** 这些子代理都会被调用
- **且** 浏览器路径测试员收到 relevance check prompt 后返回 `relevant:false`
- **且** 浏览器路径测试员不执行浏览器探索
- **且** `required:true` 加 `relevant:false` 不阻断 QA 主阶段
- **且** 最终 run 完成为 `done`
- **测试**：`docs/changes/24-树状简化wo配置/tests/test_subagent_relevance_contract.sh`
- **真实数据来源**：测试创建临时 active change、fake Pi 子代理和 fake Codex 主阶段产物
- **入口路径**：`wo run --change 1-纯CLI配置变更 --json`
- **关键断言**：全部默认子代理调用被记录；浏览器 artifact 含 `relevant:false`；最终 `state.json.status == done`
- **剩余风险**：真实浏览器操作由后续 Web 项目业务测试覆盖，本场景只证明无关路径不浪费浏览器会话
