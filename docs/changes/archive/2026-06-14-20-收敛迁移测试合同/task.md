# 文件目的

本文件拆解收敛迁移测试合同的执行步骤。

## 任务

- [x] 先运行 `bash docs/changes/archive/2026-06-14-20-收敛迁移测试合同/tests/migrated-tests-contract_test.sh`，确认当前失败来自 `.gotest` 迁移层或根测试未绿。
- [x] 审计 `docs/changes/archive/2026-06-14-20-收敛迁移测试合同/tests/app/*.gotest`，记录保留、改写、删除决策。
- [x] 将仍有效测试迁入 `internal/app/*_test.go` 或其它真实 Go 测试包。
- [x] 删除过期 `.gotest` 文件和临时迁移包装，或确保仓库不再有 `.gotest` 输入。
- [x] 运行 `go test ./...` 和本提案测试脚本，确认根门禁稳定。

## 验证条件

- [x] 本提案测试脚本通过。
- [x] `go test ./...` 通过。
- [x] `git status --short` 中不再出现未处理的迁移测试残留。
