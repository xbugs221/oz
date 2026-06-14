# 31-拆分status-view渲染边界

当前 `internal/app/status_view.go` 同时负责状态视图构建、阶段耗时计算、终端紧凑渲染、stale lock 判断和显示宽度处理。后续修改 `wo status` 或 `wo watch` 时，很容易在一个 1000 行以上文件里误伤不相关逻辑。

本次交付目标是按职责拆分 status view 相关代码，并保持现有 `go test ./internal/app` 中 status/watch 行为不变。非目标是不改 CLI 文案、不重新设计 status 输出格式、不调整 JSON 契约。

执行阶段默认先运行 `bash docs/changes/31-拆分status-view渲染边界/tests/status_view_boundary_test.sh`，确认当前实现因结构边界缺失失败，再完成拆分并让该测试和相关 Go 回归测试通过。验收证据写入 `test-results/31-status-view-boundary/contract.log`。
