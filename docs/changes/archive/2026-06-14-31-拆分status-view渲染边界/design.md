# 设计：拆分 status view 渲染边界

## 技术决策

1. 保持 `internal/app` 包不变，只按文件拆分职责，避免引入跨包循环。
2. `status_view_model.go` 放置 `statusView`、`statusViewRow`、`compactStageSpec` 和 `buildStatusView` 入口。
3. `status_duration.go` 放置 `stageDurationItems`、`stageDurationSummaryLines`、`statusWorkflowWallDuration` 等时间计算。
4. `status_render_compact.go` 放置 `compactStatusLines`、列宽、中文宽度和 marker 文本渲染。
5. `status_stale.go` 放置 `humanDisplayState`、`isStaleRunningRun`。

## 取舍

本次只做边界拆分，不改变函数签名和数据结构。这样可以让历史 shell 合同和 Go 单测继续覆盖行为，同时降低重构风险。

## 风险

- 文件移动可能漏掉未导出的 helper，导致编译失败。
- 如果执行阶段顺手改文案，可能扩大影响面。验收测试必须继续跑现有 status/watch 回归。
