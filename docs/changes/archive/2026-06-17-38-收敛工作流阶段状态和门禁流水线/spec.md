## 验收矩阵

| 需求 | 场景 | required_tests | required_evidence | 剩余风险 |
| --- | --- | --- | --- | --- |
| 阶段状态语义收敛 | 阶段和状态有单一解析入口 | stage-gate-pipeline-contract | stage-gate-pipeline-log | 不验证所有历史 fixture，仅验证公开阶段和核心状态 |
| 主阶段门禁流水线统一 | loop 和 DAG 复用同一主阶段门禁 | stage-gate-pipeline-contract | stage-gate-pipeline-log | 静态约束无法证明所有运行分支，只防止核心重复逻辑回流 |
| 兼容现有输出 | status 和 runner JSON 保持原字符串输出 | stage-gate-pipeline-contract | stage-gate-pipeline-log | 详细 UI 排版仍由既有 Go 测试覆盖 |

### 需求：阶段状态语义收敛

系统必须提供单一阶段解析和状态规范化入口，避免核心路径继续散落裸字符串判断。

#### 场景：阶段和状态有单一解析入口

- **测试**：`docs/changes/38-收敛工作流阶段状态和门禁流水线/tests/stage_gate_pipeline_contract_test.sh`
- **真实数据来源**：当前仓库 `internal/app` 源码和 `go test ./internal/app`。
- **入口路径**：shell 契约测试从仓库根目录执行。
- **关键断言**：源码中存在阶段解析 helper、运行状态 helper，并且 `go test ./internal/app` 通过。
- **剩余风险**：测试不要求导出 API 名称；执行阶段可选择内部命名，但必须满足契约脚本的边界检查。

### 需求：主阶段门禁流水线统一

系统必须让 loop 路径和 DAG node 路径复用同一主阶段门禁流水线，确保 artifact、acceptance、validation 和 advance 顺序一致。

#### 场景：loop 和 DAG 复用同一主阶段门禁

- **测试**：`docs/changes/38-收敛工作流阶段状态和门禁流水线/tests/stage_gate_pipeline_contract_test.sh`
- **真实数据来源**：`internal/app/engine_run.go`、`internal/app/node.go`、新增流水线文件和现有 Go 单测。
- **入口路径**：shell 契约测试检查源码边界并运行 Go 测试。
- **关键断言**：`runLoop` 与 `nodeRunStage` 都调用同一流水线；`nodeRunStage` 内不再直接串联 acceptance preflight、acceptance run、validation、mark completed、advance。
- **剩余风险**：测试不模拟所有失败组合；这些组合由现有 `go_dag_execution_context_test.go` 和新增执行阶段单测补足。

### 需求：兼容现有输出

系统必须保持 `state.json`、status/watch 和 runner JSON 的公开字符串输出兼容。

#### 场景：status 和 runner JSON 保持原字符串输出

- **测试**：`docs/changes/38-收敛工作流阶段状态和门禁流水线/tests/stage_gate_pipeline_contract_test.sh`
- **真实数据来源**：现有 `internal/app` status 和 runner Go 测试。
- **入口路径**：契约测试运行 `go test ./internal/app`。
- **关键断言**：引入类型化 helper 后，现有 status、watch、runner JSON、DAG 和 validation 测试仍通过。
- **剩余风险**：不覆盖用户本机历史 runtime state 的所有变体；执行阶段应保留旧字符串兼容分支。
