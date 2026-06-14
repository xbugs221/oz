# 任务

- [x] 1.1 先运行 `bash docs/changes/28-拆分wo命令分发边界/tests/test_wo_command_dispatch_boundary_contract.sh`，确认失败点是命令边界文件缺失或 `app.go` 仍有大 switch。
- [x] 1.2 新增 `command_dispatch.go`，迁出 repo 命令分发和 handler。
- [x] 1.3 新增 `interactive.go`，迁出无参数交互菜单和 change 选择流程。
- [x] 1.4 新增 `planning.go`，迁出 planning prompt、工具启动和 session id 捕获。
- [x] 1.5 精简 `app.go`，只保留早期命令、repo/context/engine 装配和委托调用。
- [x] 1.6 运行 `go test ./internal/app ./cmd/oz -count=1` 和合同测试。
- [x] 1.7 人工核对非 git 目录下 `wo --help`、`wo --version`、全局 `wo config --global` 仍不强制要求 repo。
