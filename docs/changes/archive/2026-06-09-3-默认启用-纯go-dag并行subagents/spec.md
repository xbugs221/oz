# 规格

## 验收矩阵

| 需求 | 场景 | required_tests | required_evidence |
| --- | --- | --- | --- |
| 默认纯 Go DAG engine | 默认 run 不依赖 Dagu CLI | `default-go-dag-run-contract` | `default-go-dag-run-log` |
| 默认 parallel subagents | 默认配置启用 parallel 并产出并行节点 | `go-dag-graph-status-contract` | `go-dag-graph-status-log` |
| 人类 status 清晰可读 | status 展示 engine、主阶段和并行成员 | `default-go-dag-run-contract`, `go-dag-graph-status-contract` | `default-go-dag-run-log`, `go-dag-graph-status-log` |
| graph 由 wo 导出 | Mermaid 图展示 fan-out/fan-in | `go-dag-graph-status-contract` | `go-dag-graph-status-log` |
| JSON contract 兼容 | runner JSON 不新增 parallel 结构字段 | `default-go-dag-run-contract` | `default-go-dag-run-log` |

### 需求：默认纯 Go DAG engine

系统必须把默认 `wo run --change <change> --json` 执行路径改为内嵌纯 Go DAG engine。缺少或失败的 `dagu` CLI 不得影响默认运行。

#### 场景：默认 run 不依赖 Dagu CLI

- **对应测试**：`docs/changes/archive/2026-06-09-3-默认启用-纯go-dag并行subagents/tests/test_default_go_dag_run_contract.sh`
- **真实数据来源**：测试在临时 git 仓库中创建真实 active change，包含 `proposal.md`、`design.md`、`spec.md`、`task.md`、`acceptance.json` 和 `docs/changes/archive/2026-06-09-3-默认启用-纯go-dag并行subagents/tests/`。
- **入口路径**：构建当前 checkout 的 `cmd/wo` 和 `cmd/oz`，运行 `wo run --change 1-默认go-dag --json`。
- **关键断言**：PATH 中放置会失败的 fake `dagu`，默认运行仍成功且没有调用 `dagu`；最终 run state 包含 `engine: go-dag`；`workflow_config.parallel.enabled` 为 `true`；人类 `wo status -w1` 显示 `引擎 go-dag` 和并行摘要。
- **剩余风险**：fake agent 只覆盖最小 execution/archive 路径，不验证真实 LLM 输出质量。

### 需求：默认 parallel subagents

系统必须默认启用 parallel subagents。planning context、implementation context、review 和 QA group 必须出现在默认配置和 DAG graph 中；启用后必须由 DAG 节点真实执行成员并 fan-in 到既有 parallel artifact。

#### 场景：默认配置启用 parallel 并产出并行节点

- **对应测试**：`docs/changes/archive/2026-06-09-3-默认启用-纯go-dag并行subagents/tests/test_go_dag_graph_status_contract.sh`
- **真实数据来源**：测试在临时 git 仓库中调用真实 `wo config` 生成默认 `wo.yaml`，并调用真实 `wo graph --change demo --format mermaid`。
- **入口路径**：`wo config`、`wo graph --change demo --format mermaid`。
- **关键断言**：生成的 `wo.yaml` 包含 `engine: go-dag` 和 `parallel.enabled: true`；Mermaid 图包含 `planning_context`、`implementation_context`、`review`、`qa` 的 fan-out/fan-in 节点；图中不要求 Dagu CLI。
- **剩余风险**：该测试验证图和配置，不直接执行每个成员；成员执行由默认 run 契约测试覆盖。

### 需求：人类 status 清晰可读

系统必须让 `wo status` 成为默认可观测入口，用户无需进入 run 目录即可理解当前 engine、总进度、主阶段和每个并行成员状态。

#### 场景：status 展示 engine、主阶段和并行成员

- **对应测试**：`docs/changes/archive/2026-06-09-3-默认启用-纯go-dag并行subagents/tests/test_default_go_dag_run_contract.sh` 和 `docs/changes/archive/2026-06-09-3-默认启用-纯go-dag并行subagents/tests/test_go_dag_graph_status_contract.sh`
- **真实数据来源**：测试使用真实 run state、真实 `parallel-*.json` artifact 和真实 `wo status -w1` 输出。
- **入口路径**：`wo status -w1`。
- **关键断言**：status 输出包含 `引擎 go-dag`、当前 workflow 别名、主阶段行、并行 group 摘要、成员名称和成员状态；缺失或非法 parallel artifact 不得显示 success。
- **剩余风险**：输出具体排版允许执行阶段微调，但必须保留这些语义关键词和成员明细。

### 需求：graph 由 wo 导出

系统必须继续由 `wo` 自己导出 DAG 图，不能把用户可见图绑定到执行库或外部 Web UI。

#### 场景：Mermaid 图展示 fan-out/fan-in

- **对应测试**：`docs/changes/archive/2026-06-09-3-默认启用-纯go-dag并行subagents/tests/test_go_dag_graph_status_contract.sh`
- **真实数据来源**：真实 `wo graph --change demo --format mermaid` 输出。
- **入口路径**：`wo graph`。
- **关键断言**：Mermaid 输出包含 subagent 节点、fan-in 节点、主阶段节点以及 review/QA/archive gate；同一份 graph spec 应能用于 DAG 构建。
- **剩余风险**：测试不校验 Mermaid 布局美观，只校验图语义完整。

### 需求：JSON contract 兼容

系统必须保持 `wo status --run-id <run-id> --json` 的 runner contract 兼容。并行摘要和可读图只进入人类输出，不进入现有 JSON DTO。

#### 场景：runner JSON 不新增 parallel 结构字段

- **对应测试**：`docs/changes/archive/2026-06-09-3-默认启用-纯go-dag并行subagents/tests/test_default_go_dag_run_contract.sh`
- **真实数据来源**：默认 run 完成后读取真实 `wo status --run-id <run-id> --json` 输出。
- **入口路径**：`wo status --run-id <run-id> --json`。
- **关键断言**：JSON 仍包含 `run_id`、`change_name`、`status`、`stage`、`stages`、`paths`、`sessions`、`error`；不得包含 `parallel`、`parallel_status`、`parallel_summary` 或 `members`。
- **剩余风险**：如执行阶段决定向 JSON 添加 `engine`，必须先确认 runner contract 是否允许扩展；本提案默认不要求添加。
