# 任务

- [x] 先运行 `bash docs/changes/34-拆分ozcli命令边界/tests/ozcli_boundary_test.sh`，确认当前实现失败于结构边界缺失。
- [x] 创建 `cli.go`，移动 CLI 入口、分发和 help。
- [x] 创建 `cmd_install.go`，移动 install 命令。
- [x] 创建 `cmd_change.go`，移动 list/create/status 和 change 状态 payload。
- [x] 创建 `cmd_validate.go`，移动 validate 和 acceptance/runtime artifact policy。
- [x] 创建 `cmd_archive.go`，移动 archive 和任务完成检查。
- [x] 运行 `go test ./internal/ozcli` 和本提案契约测试。
