# 文件目的

本文件拆解共享验收 producer 追溯逻辑的实现步骤。

## 任务

- [x] 先运行 `bash docs/changes/21-共享验收证据追溯逻辑/tests/shared-producer-contract_test.sh`，确认当前重复实现会触发结构断言失败。
- [x] 在 `internal/acceptance` 抽取 producer 追溯纯函数和 findings API。
- [x] 给 `internal/acceptance` 增加真实文件和 wrapper 场景测试。
- [x] 修改 `cmd/oz` 的 `validateAcceptanceFiles` 调用共享 API。
- [x] 修改 `internal/app` 的 `acceptancePreflightFindings` 调用共享 API。
- [x] 运行本提案脚本、`go test ./cmd/oz ./internal/app ./internal/acceptance`。

## 验证条件

- [x] 两个入口没有重复 producer helper 定义。
- [x] 共享包测试覆盖 producer 追溯核心路径。
- [x] `oz validate` 和 `wo` 预检仍能报告缺失 producer。
