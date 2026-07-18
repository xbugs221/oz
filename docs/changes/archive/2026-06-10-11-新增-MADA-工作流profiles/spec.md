# 规格

## 验收矩阵

| 需求 | 场景 | 测试 | 证据 | 风险 |
| --- | --- | --- | --- | --- |
| 默认工作流模板必须脱离 Go 硬编码 | 默认 profile 由内置 YAML 模板生成 | `profile-templates-externalized` | `profile-templates-externalized-log` | 只要求内置模板文件可替换并随二进制编译，不要求运行时加载任意外部模板 |
| MADA profile 可直接生成标准配置 | 三个 MADA profile 均能生成可加载的 `wo.yaml` | `mada-profiles-config` | `mada-profiles-config-log` | 不执行真实 agent，只验证配置和图入口 |
| MADA profile 可直接生成标准配置 | decision profile 包含决策评审所需角色 | `mada-profiles-config` | `mada-profiles-config-log` | 只验证角色合同，不验证最终推荐质量 |
| Profile 可发现且错误明确 | `wo config --list-profiles` 输出全部 profile | `mada-profile-discovery` | `mada-profile-discovery-log` | 输出格式只要求人类可读，不承诺 JSON |
| Profile 可发现且错误明确 | 未知 profile 返回错误并提示可用名称 | `mada-profile-discovery` | `mada-profile-discovery-log` | 不覆盖本地化语言以外的错误消息 |

### 需求：默认工作流模板必须脱离 Go 硬编码

系统必须先把当前默认 `wo.yaml` 生成逻辑中的并行 subagent 描述、角色 purpose、profile 提示词配置从 Go 字符串拼接中剥离到独立内置 YAML 模板文件，维护方式应类似 `prompts-template/*.md`。

#### 场景：默认 profile 由内置 YAML 模板生成

- **给定** 当前仓库源码
- **当** 执行阶段实现本提案
- **则** 仓库必须包含 `profiles-template/default.yaml`
- **并且** `profiles-template/default.yaml` 必须保存默认 `wo config` 当前输出语义，包括 `parallel.enabled`、`planning_context`、`implementation_context`、`review`、`qa` 和默认 subagent 角色描述
- **并且** `profiles-template/mada-code.yaml`、`profiles-template/mada-decision.yaml`、`profiles-template/mada-research.yaml` 必须使用同一模板机制维护
- **并且** Go 源码不得继续硬编码默认 subagent 角色名和 purpose 文本，例如 `需求分析员`、`代码库侦察员`、`外部资料研究员`
- **并且** 默认 `wo config` 不带 `--profile` 时仍生成与现有默认 profile 等价的标准 `wo.yaml`
- **测试**：`docs/changes/archive/2026-06-10-11-新增-MADA-工作流profiles/tests/test_profile_templates_externalized_contract.sh`
- **真实数据来源**：测试检查真实源码文件，构建真实 `wo` 二进制，并在临时 git 仓库中运行默认 `wo config`
- **入口路径**：新的 profile 模板目录及 embed 包、`internal/app/config.go` 的默认配置生成、`LoadWorkflowConfig`
- **关键断言**：模板文件存在且包含业务角色；Go 源码不再承载默认角色文本；默认 `wo config` 仍可生成和加载标准配置
- **剩余风险**：第一版不提供用户运行时指定任意 profile 文件路径的能力，替换内置模板后需要重新构建二进制

### 需求：MADA profile 可直接生成标准配置

系统必须允许用户通过 `wo config --profile <name>` 生成可试用的 MADA 工作流配置，且输出仍是标准 `wo.yaml`。

#### 场景：三个 MADA profile 均能生成可加载的 `wo.yaml`

- **给定** 一个临时 git 仓库，仓库内没有 `wo.yaml`
- **当** 用户分别运行 `wo config --profile mada-code`、`wo config --profile mada-decision`、`wo config --profile mada-research`
- **则** 每次都必须生成 `wo.yaml`
- **并且** 生成内容必须来自 `profiles-template/<profile>.yaml` 对应的内置模板，而不是在 Go 源码中拼接 profile YAML
- **并且** YAML 中必须启用 `parallel.enabled`
- **并且** 必须包含 `planning_context`、`implementation_context`、`review`、`qa` 四个并行组
- **并且** `review` 和 `qa` 必须是 `gate_input`，`planning_context` 和 `implementation_context` 必须是 `advisory`
- **并且** `wo graph --change 11-演示 --format json` 必须能成功读取该配置并输出含 subagent/fanin 节点的图
- **测试**：`docs/changes/archive/2026-06-10-11-新增-MADA-工作流profiles/tests/test_mada_profiles_config_contract.sh`
- **真实数据来源**：测试构建真实 `wo` 二进制，在临时 git 仓库中运行真实 CLI 写入和读取 `wo.yaml`
- **入口路径**：`internal/app/app.go` 的 `wo config` 解析，`internal/app/config.go` 的 profile YAML 生成和加载，`internal/app/graph.go` 的并行组展开
- **关键断言**：三类 profile 都生成标准配置；四类并行组模式正确；graph JSON 能加载并包含 MADA 角色节点
- **剩余风险**：该场景不启动真实 agent，避免创建阶段依赖外部模型

#### 场景：decision profile 包含决策评审所需角色

- **给定** 一个临时 git 仓库
- **当** 用户运行 `wo config --profile mada-decision`
- **则** 生成的 `wo.yaml` 必须包含面向技术选型的角色：`需求澄清员`、`约束建模员`、`候选方案研究员`、`反方评审员`、`运维部署评审员`、`学习路线评审员`、`证据审计员`
- **并且** `wo graph --change 11-决策演示 --format json` 必须展示这些 review 子代理节点
- **测试**：`docs/changes/archive/2026-06-10-11-新增-MADA-工作流profiles/tests/test_mada_profiles_config_contract.sh`
- **真实数据来源**：同一份由真实 CLI 生成的 `wo.yaml`
- **入口路径**：profile 模板、`LoadWorkflowConfig` 和 `BuildWorkflowSpec`
- **关键断言**：decision profile 不退化为默认代码审查角色；角色名称能在配置和图中被审计
- **剩余风险**：该测试不评价具体推荐答案质量，答案质量由使用 profile 的下游 change 自己的 `acceptance.json` 约束

### 需求：Profile 可发现且错误明确

系统必须让用户能发现可用 profile，并在输入错误 profile 时得到明确反馈。

#### 场景：`wo config --list-profiles` 输出全部 profile

- **给定** 用户在任意 git 仓库中
- **当** 用户运行 `wo config --list-profiles`
- **则** 输出必须包含 `default`、`mada-code`、`mada-decision`、`mada-research`
- **并且** 每个 MADA profile 必须带有中文用途说明
- **并且** 该命令不得写入 `wo.yaml`
- **测试**：`docs/changes/archive/2026-06-10-11-新增-MADA-工作流profiles/tests/test_mada_profile_discovery_contract.sh`
- **真实数据来源**：测试在临时 git 仓库中运行真实 `wo` 二进制
- **入口路径**：`internal/app/app.go` 的 `wo config --list-profiles` 分支和 profile registry
- **关键断言**：profile 可发现；list 命令无写文件副作用
- **剩余风险**：第一版只要求 human 输出，不要求 JSON 输出

#### 场景：未知 profile 返回错误并提示可用名称

- **给定** 一个临时 git 仓库
- **当** 用户运行 `wo config --profile not-real`
- **则** 命令必须非零退出
- **并且** stderr 必须包含未知 profile 名称
- **并且** stderr 必须提示 `mada-code`、`mada-decision`、`mada-research` 中至少一个可用名称
- **并且** 不得写入 `wo.yaml`
- **测试**：`docs/changes/archive/2026-06-10-11-新增-MADA-工作流profiles/tests/test_mada_profile_discovery_contract.sh`
- **真实数据来源**：测试运行真实 CLI 的错误路径
- **入口路径**：`wo config` 参数解析和 profile registry 错误处理
- **关键断言**：错误明确且无配置文件副作用
- **剩余风险**：错误文案可以调整，但必须包含输入名和可用 profile 名称
