# 任务

- [x] 先运行 `bash docs/changes/archive/2026-06-14-31-拆分status-view渲染边界/tests/status_view_boundary_test.sh`，确认当前实现失败于结构边界缺失。
- [x] 创建 `status_view_model.go`，移动 status view 类型、阶段规格和 view 构建入口。
- [x] 创建 `status_duration.go`，移动阶段耗时和 workflow wall time 计算。
- [x] 创建 `status_render_compact.go`，移动紧凑终端渲染、列宽和显示宽度 helper。
- [x] 创建 `status_stale.go`，移动 stale running run 的显示判断。
- [x] 保持现有 status/watch 行为，运行 `go test ./internal/app` 和本提案契约测试。
