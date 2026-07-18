# 任务

- [x] 1.1 先运行 `bash docs/changes/archive/2026-06-14-27-统一状态展示视图入口/tests/test_status_view_unification_contract.sh`，确认失败点是 `app.go` 仍保留展示 helper。
- [x] 1.2 梳理 `app.go` 中 checklist、session、duration 和 status marker helper 的调用方。
- [x] 1.3 将展示数据计算迁入 `status_view.go`，将文本输出迁入或保留在 `status_render.go`。
- [x] 1.4 修改执行进度 `printProgress` 走共享 status view renderer。
- [x] 1.5 运行 status/view/watch/runner JSON 聚焦回归和 `go test ./internal/app -count=1`。
- [x] 1.6 人工检查 `wo status --json` 没有新增 human-only 字段或中文摘要泄漏。
