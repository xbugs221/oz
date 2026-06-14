# 统一状态展示视图入口

本提案收敛 `wo status`、`wo watch`、runner JSON 和执行进度 checklist 的展示计算，让状态展示统一通过 `status_view` 的 view model，再由不同 renderer 输出人类文本或 JSON。

交付目标：

- `internal/app/app.go` 不再承载 checklist、session 分组、耗时汇总等状态展示计算。
- `status_view.go` 作为状态展示的唯一数据模型入口，`status_render.go` 只负责格式化输出。
- 现有 human status、watch、runner JSON 观测行为保持稳定。

非目标：

- 不修改 `State` 持久化结构。
- 不改变 `wo status --json` 的机器字段含义。
- 不重新设计中文展示文案，只迁移和去重展示计算。

验收入口：

- `bash docs/changes/27-统一状态展示视图入口/tests/test_status_view_unification_contract.sh`
- `go test ./internal/app -run 'TestStatusView|TestPrintHumanStatus|TestWatch|TestRunner|TestCompactStatus|TestHumanStatus' -count=1`

执行阶段默认上下文：先读 `internal/app/status_view.go`、`internal/app/status_render.go`、`internal/app/status_view_test.go` 和 `internal/app/app.go` 中的旧 checklist helper，保持输出合同再迁移。
