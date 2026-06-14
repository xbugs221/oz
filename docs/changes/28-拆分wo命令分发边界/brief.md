# 拆分 wo 命令分发边界

本提案把 `internal/app/app.go` 中的 `Run` 大型命令分发、交互菜单和规划入口拆成独立边界，降低新增或修改 `wo` 命令时的回归风险。

交付目标：

- `Run` 只负责基础装配：处理无需 repo 的早期命令、解析 repo、创建 context/engine、委托命令分发。
- JSON runner 命令、人类命令、交互流程和规划流程分别进入独立文件。
- `wo run`、`wo resume`、`wo batch`、`wo restart`、`wo status`、`wo clean`、`wo watch` 等现有命令行为保持不变。

非目标：

- 不新增 CLI 命令。
- 不改变命令参数、错误文案或 JSON 输出。
- 不拆 `cmd/oz` 的独立 CLI。

验收入口：

- `bash docs/changes/28-拆分wo命令分发边界/tests/test_wo_command_dispatch_boundary_contract.sh`
- `go test ./internal/app ./cmd/oz -count=1`

执行阶段默认上下文：先读 `internal/app/app.go`、`internal/app/restart.go`、`internal/app/batch.go`、`internal/app/status_render.go` 和 `cmd/oz/main_test.go`，优先保持命令面稳定。
