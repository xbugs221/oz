# 规格

## 验收矩阵

| 场景 | required_tests | required_evidence |
| --- | --- | --- |
| `wo graph` 不再暴露 Dagu 格式 | `contract-no-dagu-graph-engine` | `runtime-no-dagu-graph-engine` |
| `workflow.engine` 只接受 `go-dag` | `contract-no-dagu-graph-engine` | `runtime-no-dagu-graph-engine` |
| 默认 subagent tool 为 `pi` | `contract-default-subagent-pi` | `runtime-default-subagent-pi` |
| 默认最大迭代数为 5 | `contract-compact-chinese-graph-iteration-limit` | `runtime-compact-chinese-graph` |
| `wo graph` 输出紧凑中文图 | `contract-compact-chinese-graph-iteration-limit` | `runtime-compact-chinese-graph` |
| subagent artifact 类型错误会 resume 原会话修正 | `contract-subagent-artifact-retry` | `runtime-subagent-artifact-retry` |

### 需求：graph 和 engine 去 Dagu 化

系统必须让用户可见 workflow engine 合同只保留内嵌 `go-dag`，并从 `wo graph` 和 `wo run` 的公开入口移除 Dagu 执行器残留。

#### 场景：wo graph 不再暴露 Dagu 格式

- **测试文件**：`docs/changes/4-清理-Dagu-残留并统一-pi-subagents/tests/test_no_dagu_graph_engine_contract.sh`
- **真实数据来源**：测试脚本构造临时 git 仓库，使用源码构建出的真实 `wo` 二进制执行 `wo graph --change demo --format json|mermaid|dagu`
- **入口路径**：`wo graph --change demo --format <format>`
- **关键断言**：
  - `json` 和 `mermaid` 仍能成功输出 workflow graph
  - `json` 和 `mermaid` 输出不包含 Dagu 相关公开概念
  - `--format dagu` 返回非零退出
  - 错误提示不得继续把 `dagu` 列为可选 graph format
- **剩余风险**：该测试不扫描所有源码注释，只验证用户可见 graph 行为。

#### 场景：workflow.engine 只接受 go-dag

- **测试文件**：`docs/changes/4-清理-Dagu-残留并统一-pi-subagents/tests/test_no_dagu_graph_engine_contract.sh`
- **真实数据来源**：同一个临时 git 仓库写入不同 `wo.yaml`，分别使用 `engine: legacy`、`engine: dagu` 和 `wo run --engine dagu --json` 验证公开入口
- **入口路径**：`wo graph --change demo --format json`、`wo run --change demo --engine dagu --json`
- **关键断言**：
  - `workflow.engine: legacy` 被拒绝
  - `workflow.engine: dagu` 被拒绝
  - `wo run --engine dagu` 不检查或调用 Dagu CLI
  - 失败信息引导用户使用唯一支持的 `go-dag`
- **剩余风险**：历史 run state 中旧 engine 值的只读展示兼容由实现阶段按最小风险处理。

### 需求：默认 parallel subagent tool 使用 pi

系统必须让新项目通过 `wo config` 生成的默认 parallel planning/implementation subagent 成员使用 `tool: pi`，避免继续传播过时的 `opencode` 默认值。

#### 场景：默认 subagent tool 为 pi

- **测试文件**：`docs/changes/4-清理-Dagu-残留并统一-pi-subagents/tests/test_default_subagent_pi_contract.sh`
- **真实数据来源**：测试脚本构造没有 `wo.yaml` 的临时 git 仓库，运行源码构建出的真实 `wo config`
- **入口路径**：`wo config`、`wo graph --change demo --format json`
- **关键断言**：
  - 生成的 `wo.yaml` 包含 `engine: go-dag`
  - planning/implementation parallel subagent 成员至少生成 5 个 `tool: pi`
  - 生成的 `wo.yaml` 不包含 `tool: opencode`
  - 默认 graph 仍包含 planning/implementation subagent 节点
- **剩余风险**：该测试不要求 review/QA gate_input 成员写入 tool，因为它们当前没有单独 backend 线索。

### 需求：默认迭代预算和 graph 展示保持克制

系统必须把默认 `max_review_iterations` 降到 `5`，并让 `wo graph --format mermaid` 用紧凑中文状态图表达 review/QA/fix 循环，不得按最大轮次重复绘制一批结构相同的节点。

#### 场景：默认最大迭代数为 5

- **测试文件**：`docs/changes/4-清理-Dagu-残留并统一-pi-subagents/tests/test_compact_chinese_graph_and_iteration_limit.sh`
- **真实数据来源**：测试脚本构造没有 `wo.yaml` 的临时 git 仓库，运行源码构建出的真实 `wo config`
- **入口路径**：`wo config`
- **关键断言**：
  - 生成的默认 `wo.yaml` 包含 `max_review_iterations: 5`
  - 默认配置仍包含 `engine: go-dag`
  - 默认配置不再生成 `max_review_iterations: 30`
- **剩余风险**：用户显式配置更大轮次时的处理策略由实现阶段决定，但默认值必须是 5。

#### 场景：wo graph 输出紧凑中文图

- **测试文件**：`docs/changes/4-清理-Dagu-残留并统一-pi-subagents/tests/test_compact_chinese_graph_and_iteration_limit.sh`
- **真实数据来源**：同一个临时 git 仓库使用默认 `wo.yaml` 执行真实 `wo graph --change demo --format mermaid`
- **入口路径**：`wo graph --change demo --format mermaid`
- **关键断言**：
  - Mermaid 图不包含 `review_2`、`qa_2`、`fix_2` 这类逐轮重复节点
  - Mermaid 可见标签不包含 `subagent:`、`fan-in`、`planning_context`、`implementation_context`
  - Mermaid 图保留中文 subagent 名称，如“需求分析员”“代码库侦察员”“外部资料研究员”
  - Mermaid 图表达最多 5 轮的审核修复预算
- **剩余风险**：JSON graph 是否继续作为执行调度用完整 spec 保留，由实现阶段根据 `go-dag` 调度需要决定；本场景只约束人类可读 Mermaid 图。

### 需求：go-dag subagent artifact hook 自动修正格式错误

系统必须在 go-dag subagent member 正常退出并写出 `SUBAGENT_OUTPUT` 后立即执行 artifact schema gate。若产物字段类型或结构不符合 `ParallelMemberResult` 合同，系统必须 resume 对应 subagent 会话，要求只重写该 member artifact，最多 3 次；不得把这种局部格式错误直接提升为整个 workflow failed。

#### 场景：subagent artifact 类型错误会 resume 原会话修正

- **测试文件**：`docs/changes/4-清理-Dagu-残留并统一-pi-subagents/tests/test_subagent_artifact_retry_contract.sh`
- **真实数据来源**：测试脚本构造临时 git 仓库和 fake `pi`，使用源码构建出的真实 `wo` 二进制运行默认 go-dag workflow；fake `pi` 第一次为同一个 subagent 写出对象数组 `evidence`，第二次在收到 schema 修正提示和原 session id 后写出字符串数组 `evidence`
- **入口路径**：`wo run --change <change> --json`
- **关键断言**：
  - 默认 go-dag run 不调用 `dagu` 可执行文件
  - fake subagent 第一次写出的 `evidence: [{"source": "..."}]` 被 schema gate 拦截
  - 第二次修正调用使用同一个 subagent session id，而不是新建无上下文会话
  - 修正 prompt 包含失败字段、期望类型、`SUBAGENT_OUTPUT` 路径和“只重写 artifact”的约束
  - 最终 member artifact 中 `evidence` 为字符串数组，且 fan-in 能继续读取该成员产物
  - 若连续 3 次仍不合格，错误必须包含成员名、字段名、期望类型和 artifact 路径
- **剩余风险**：该测试使用 fake agent 验证 session resume 语义和 artifact 修正闭环，不依赖真实 LLM 是否会遵守提示词。
