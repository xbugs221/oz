# 文件目的

本提案统一 human status、watch 和 runner JSON 的状态展示视图模型，减少同一运行态在不同入口输出不一致。

## 用户问题

状态展示逻辑散在 `status_view.go`、`app.go`、batch/watch helper 和 runner DTO 中。每次调整 status 或 watch 输出，都容易漏改另一个入口。

## 交付目标

- 让 human status、watch、runner JSON 都从共享 `statusView` 或 renderer 派生。
- 将 watch/status 文本格式化移出大型 `app.go`。
- 保持当前紧凑中文输出和 JSON 合同稳定。

## 非目标

- 不重新设计状态文案。
- 不改变 run/batch 状态文件 schema。
- 不引入 TUI 框架。

## 验收入口

执行 `bash docs/changes/23-统一状态展示视图模型/tests/status-view-contract_test.sh`。

## 执行阶段默认上下文

先读取 `internal/app/status_view.go`、`internal/app/app.go` 的 `printHumanStatus`、`runWatch`、`watchBatchStatusLines`、`runProposalStatusLines`，以及 status/batch 相关测试。
