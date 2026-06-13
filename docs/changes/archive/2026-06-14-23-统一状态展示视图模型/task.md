# 文件目的

本文件拆解统一状态展示视图模型的执行步骤。

## 任务

- [x] 先运行 `bash docs/changes/23-统一状态展示视图模型/tests/status-view-contract_test.sh`，确认当前展示 helper 仍散在 `app.go`。
- [x] 新建 `internal/app/status_render.go` 或等价文件。
- [x] 将 watch/run/batch 状态行渲染迁到 renderer。
- [x] 让 `printHumanStatus`、`watchStatusLines` 和 runner JSON 复用同一视图模型。
- [x] 迁移或补充 status/batch 回归测试。
- [x] 运行本提案脚本、`go test ./internal/app` 和根测试。

## 验证条件

- [x] `app.go` 不再承载状态文本拼接 helper。
- [x] status/watch/JSON 关键测试通过。
- [x] 输出文案没有无关美化或合同漂移。
