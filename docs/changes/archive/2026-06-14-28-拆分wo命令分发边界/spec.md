# 规格：拆分 wo 命令分发边界

## 验收矩阵

| 场景 | 测试 | 证据 | 风险 |
| --- | --- | --- | --- |
| wo 命令分发从 app.go 拆出且命令面稳定 | `wo-command-dispatch-boundary-contract` | `wo-command-dispatch-boundary-log` | 不逐个运行所有 shell 业务合同，依赖 Go 命令测试覆盖主命令面 |

### 需求：wo 命令分发拆分

系统必须把 `wo` 的 repo 命令分发、交互菜单和规划入口从 `app.go` 拆出到独立文件，同时保持现有 CLI 命令行为。

#### 场景：wo 命令分发从 app.go 拆出且命令面稳定

- **测试文件**：`docs/changes/archive/2026-06-14-28-拆分wo命令分发边界/tests/test_wo_command_dispatch_boundary_contract.sh`
- **真实数据来源**：仓库当前 Go CLI 源码、`internal/app` Go 单测和 `cmd/oz` 命令测试构造的真实临时项目。
- **入口路径**：从仓库根目录运行 shell 合同测试，测试内部执行 `go test ./internal/app ./cmd/oz -count=1`。
- **关键断言**：
  - `internal/app/command_dispatch.go`、`internal/app/interactive.go`、`internal/app/planning.go` 必须存在。
  - `app.go` 不再直接包含 repo 命令的大 switch case。
  - `internal/app` 和 `cmd/oz` Go 单测通过，证明命令面和相关工作流入口保持稳定。
- **剩余风险**：合同测试不覆盖每个 shell 端到端场景；执行阶段可按影响面补跑 `tests/specs/codex-workflow-cli` 中相关脚本。
