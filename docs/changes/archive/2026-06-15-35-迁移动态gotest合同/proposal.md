# 提案：迁移动态 gotest 合同

## 背景

历史 shell 合同为了把包内 Go 测试临时注入 `internal/app`，会在 `docs/changes/archive/2026-06-15-35-迁移动态gotest合同/tests/app` 下生成 `.gotest` 文件，再由 `migrated_app_suite_test.go` 复制源码执行。这曾经便于迁移，但现在已经变成隐藏测试入口。

## 目标

- 删除 `docs/changes/archive/2026-06-15-35-迁移动态gotest合同/tests/app/migrated_app_suite_test.go` 动态 runner。
- 移除 shell 合同中 `cat > docs/changes/archive/2026-06-15-35-迁移动态gotest合同/tests/app/*.gotest` 的生成逻辑。
- 将这些断言迁移到长期 `_test.go` 文件，或改写为直接 shell 合同。
- 保持 `go test ./...` 作为可理解的主测试入口。
- 保留原动态合同覆盖的 status、parallel、go-dag、subagent 业务断言。

## 非目标

- 不删除或弱化原有业务断言。
- 不改变 `internal/app` 被测行为。
- 不引入新的动态测试格式。
