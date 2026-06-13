# 文件目的

记录本提案对 `tests/app/*.gotest` 迁移合同的审计决策，避免根目录 Go 门禁继续执行过期临时输入。

## 审计结论

`tests/app/*.gotest` 是历史迁移层输入，不再作为当前根目录 Go 门禁的事实合同保留。当前源码已有真实 Go 测试覆盖核心入口：

- `internal/app/agy_test.go`
- `internal/app/gate_state_preflight_test.go`
- `internal/app/go_dag_execution_context_test.go`
- `internal/app/status_view_test.go`
- `cmd/oz/main_test.go`
- `tests/command_surface_test.go`

其余业务合同继续由根目录和 `tests/specs/` 下的 shell 测试覆盖。部分历史 `.gotest` 仍断言旧 CLI 命令或迁移期实现细节，继续纳入 `go test ./...` 会让根门禁混入旧意图噪音。

因此本次删除 `tests/app` 下全部静态 `.gotest` 输入，并保留 `tests/app/migrated_app_suite_test.go` 作为动态 shell 合同的兼容运行器：没有 `.gotest` 时跳过，有 shell 测试临时写入 `.gotest` 时仍执行。
