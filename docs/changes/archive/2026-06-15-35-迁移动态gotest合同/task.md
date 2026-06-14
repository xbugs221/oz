# 任务

- [x] 先运行 `bash docs/changes/35-迁移动态gotest合同/tests/gotest_migration_test.sh`，确认当前实现失败于仍存在动态 `.gotest` 机制。
- [x] 盘点所有生成 `tests/app/*.gotest` 的 shell 合同，记录迁移目标。
- [x] 将包内 Go 断言迁入长期 `internal/app/*_test.go`。
- [x] 将适合端到端的断言改写为直接 shell 合同，不再写 `.gotest`。
- [x] 删除 `tests/app/migrated_app_suite_test.go` 和 `OZ_MIGRATED_APP_RUN` 依赖。
- [x] 运行本提案契约测试、`go test ./...` 和受影响 shell 合同。
