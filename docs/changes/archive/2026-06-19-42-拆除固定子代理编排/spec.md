# 规格：拆除固定子代理编排

## 验收矩阵

| 场景 | 测试 | 证据 | 风险 |
| --- | --- | --- | --- |
| 默认配置不再生成外置子代理配置 | `remove-fixed-subagents-contract` | `remove-fixed-subagents-log`、`remove-fixed-subagents-graph` | 不覆盖每个 profile；执行阶段应按影响面补查 MADA profile |
| 默认 workflow graph 不再包含 subagent/fan-in 观测节点 | `remove-fixed-subagents-contract` | `remove-fixed-subagents-log`、`remove-fixed-subagents-graph` | 不启动真实 agent；用 graph JSON 证明新 run 拓扑 |
| 主阶段 prompt 不再依赖 oz 子代理 artifact | `remove-fixed-subagents-contract` | `remove-fixed-subagents-log` | 只检查内置 prompt 模板；用户自定义 prompt 可自行保留文本 |
| 旧外置子代理配置字段明确拒绝 | `remove-fixed-subagents-contract` | `remove-fixed-subagents-log` | 不迁移用户配置文件内容，只要求错误明确 |
| 生产代码不再保留外置子代理 runner/fan-in 边界 | `remove-fixed-subagents-contract` | `remove-fixed-subagents-log` | 静态断言不证明所有死代码都移除；执行阶段仍需跑 Go 回归 |

### 需求：默认配置移除固定子代理

系统必须让新用户默认看不到 oz 外置固定子代理配置。默认配置只表达主阶段 agent、reasoning、validation 和 prompts。

#### 场景：默认配置不再生成外置子代理配置

- **测试文件**：`docs/changes/42-拆除固定子代理编排/tests/test_remove_fixed_subagents_contract.sh`
- **真实数据来源**：临时 git 仓库中运行真实 `oz flow config` 生成的 `oz-flow.yaml`。
- **入口路径**：从仓库根目录运行 shell 契约测试，测试内部构建真实 `cmd/oz` 二进制并在临时仓库调用 `flow config`。
- **关键断言**：
  - 默认 `oz-flow.yaml` 不包含 `parallel:`、`subagent_guard:` 或 `before:`。
  - 默认 `oz-flow.yaml` 不包含内置固定 helper 名称，例如 `代码库侦察员`、`目标核对审核员`、`浏览器路径测试员`。
  - 默认 `oz-flow.yaml` 仍包含 execution、review、qa、fix、archive 主阶段配置。
- **剩余风险**：本场景只检查默认 profile；执行阶段若保留 MADA profile，应同步决定是否删除其中的固定子代理并补充回归。

### 需求：工作流观测不再展示外置子代理

系统必须让默认 workflow graph 和 status/watch 的事实源不再包含 oz 外置子代理节点、fan-in 节点或 parallel artifact。主流程阶段和 gate 必须保持。

#### 场景：默认 workflow graph 不再包含 subagent/fan-in 观测节点

- **测试文件**：`docs/changes/42-拆除固定子代理编排/tests/test_remove_fixed_subagents_contract.sh`
- **真实数据来源**：同一个临时仓库中真实 `oz flow graph --change demo --format json` 输出。
- **入口路径**：契约测试内部调用真实 `cmd/oz` 二进制的 graph 命令，并解析 JSON graph。
- **关键断言**：
  - graph nodes 不包含 `type=subagent` 或 `type=fanin`。
  - graph artifacts 不包含 `parallel-*` 路径。
  - graph 仍包含 `execution`、`review_1`、`qa_1`、`fix_1`、`archive` 和 gate 节点。
- **剩余风险**：该场景不启动完整 sealed run；status/watch 的运行时展示由生产代码静态边界断言和执行阶段 Go 回归补足。

### 需求：主阶段 prompt 不依赖 oz 子代理 artifact

系统必须停止在内置 prompt 中要求主代理读取 oz 生成的 subagent/parallel artifact。阶段主代理是否派内部 sub agent 不由 oz 提示词显式强调。

#### 场景：主阶段 prompt 不再依赖 oz 子代理 artifact

- **测试文件**：`docs/changes/42-拆除固定子代理编排/tests/test_remove_fixed_subagents_contract.sh`
- **真实数据来源**：仓库内真实 `prompts-template/oz-flow-start.md`、`oz-flow-review.md`、`oz-flow-qa.md`。
- **入口路径**：契约测试直接检查内置 prompt 模板源码。
- **关键断言**：
  - execution/review/QA prompt 不包含 `subagent artifact`、`parallel-`、`ParallelContext`、`ParallelReview`、`ParallelQA`、`review helper` 或 `QA helper`。
  - prompt 仍保留主阶段必要输入，例如 `StatePath`、`AcceptancePath`、`ChangePath`、`ReviewPath` 或 `QAPath`。
- **剩余风险**：用户自定义 prompt 不是本次合同范围。

### 需求：旧外置子代理配置字段明确拒绝

系统必须拒绝旧外置子代理配置字段，避免用户以为这些字段仍会影响运行。

#### 场景：旧外置子代理配置字段明确拒绝

- **测试文件**：`docs/changes/42-拆除固定子代理编排/tests/test_remove_fixed_subagents_contract.sh`
- **真实数据来源**：契约测试创建的三个临时仓库配置，分别包含 `parallel`、`subagent_guard` 和 `stages.execution.before`。
- **入口路径**：契约测试对这些临时仓库运行真实 `oz flow graph --change demo --format json`，触发配置加载。
- **关键断言**：
  - 包含 `parallel` 的配置必须失败，并在错误中提到 `parallel` 已删除或不再支持。
  - 包含 `subagent_guard` 的配置必须失败，并在错误中提到 `subagent_guard` 已删除或不再支持。
  - 包含 `stages.execution.before` 的配置必须失败，并在错误中提到 `before` 已删除或不再支持。
- **剩余风险**：不自动改写用户旧配置；迁移动作由用户或后续工具完成。

### 需求：源码边界移除 oz 外置子代理 runner

系统必须从生产代码中移除 oz 外置子代理 runner、member artifact、fan-in 和 parallel QA gate 边界，避免形成不可见的第二套调度层。

#### 场景：生产代码不再保留外置子代理 runner/fan-in 边界

- **测试文件**：`docs/changes/42-拆除固定子代理编排/tests/test_remove_fixed_subagents_contract.sh`
- **真实数据来源**：仓库内真实 `internal/app` 生产源码和 Go 回归测试。
- **入口路径**：契约测试使用 `rg` 做源码边界断言，并运行 `go test ./internal/app ./internal/ozcli ./tests -count=1`。
- **关键断言**：
  - `internal/app` 不再包含 `nodeRunSubagent`、`nodeFanin`、`runSubagentAttempts`、`ParallelMemberResult`、`memberArtifactPath` 或 `ValidateParallelQAGate`。
  - `go test ./internal/app ./internal/ozcli ./tests -count=1` 通过，证明主流程命令面和核心状态回归保持稳定。
- **剩余风险**：静态断言不穷举所有历史命名；执行阶段应按影响面补跑现有 flow/config/status/graph shell specs。
