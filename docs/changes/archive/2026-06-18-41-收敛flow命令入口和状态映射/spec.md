# 规格：收敛 flow 命令入口和状态映射

## 验收矩阵

| 场景 | 测试 | 证据 | 风险 |
| --- | --- | --- | --- |
| 命令、拓扑和状态视图边界收敛且 CLI 行为保持稳定 | `flow-boundary-convergence-contract` | `flow-boundary-convergence-log` | 不覆盖每个历史 shell 端到端脚本；执行阶段应按影响面补跑相关 `tests/specs/codex-workflow-cli` 脚本 |

### 需求：flow 命令入口和状态映射收敛

系统必须把 `oz flow` 命令路由、workflow 阶段/并行组拓扑和公共 status 展示收敛到明确边界，避免后续重构继续在多个文件同步维护同一套业务映射，同时保持现有 CLI 行为。

#### 场景：命令、拓扑和状态视图边界收敛且 CLI 行为保持稳定

- **测试文件**：`docs/changes/archive/2026-06-18-41-收敛flow命令入口和状态映射/tests/test_flow_boundary_convergence_contract.sh`
- **真实数据来源**：仓库当前 `internal/app`、`internal/ozcli`、`tests` 生产代码和 Go 回归测试。
- **入口路径**：从仓库根目录运行 shell 契约测试，测试内部执行源码结构断言和 `go test ./internal/app ./internal/ozcli ./tests -count=1`。
- **关键断言**：
  - `internal/app/flow_command_registry.go` 存在，并定义命令 registry 结构；`app.go` 不再维护 `commandNeedsWorkflowTools` 和 repo 命令重复入口。
  - `internal/app/workflow_topology.go` 存在；graph/config/status 关键文件不再直接硬编码 `planning_context`、`implementation_context`、`before_review`、`before_qa`、`before_execution` 的映射分支。
  - 公共 status/progress 入口不再直接调用 legacy `stageChecklistLines` 系列函数，而是通过统一 status view 或适配器输出。
  - `internal/app`、`internal/ozcli` 和 `tests` Go 回归通过，证明命令面和状态输出基础行为保持稳定。
- **剩余风险**：该场景不逐个验证所有历史 shell 合同；执行阶段应根据改动影响面补跑 status/watch、graph、parallel 和 command dispatch 相关 specs。
