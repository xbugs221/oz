# 设计：统一状态展示视图入口

## 目标结构

- `status_view.go`：构建 `statusView` 和 `statusViewRow`，包含阶段汇总、session 查找、artifact 路径、duration、stale run 等计算。
- `status_render.go`：只做文本行渲染，包括 status、watch、执行进度 checklist。
- `runner_contract.go`：继续负责 runner JSON DTO，但只从 `statusView` 或 `State` 做稳定映射。
- `app.go`：不再定义状态展示 helper。

## 迁移步骤

1. 对照 `status_view_test.go` 固定当前 human/watch/JSON 行为。
2. 把 `app.go` 里的 checklist/session/duration helper 移到 `status_view.go` 或 `status_render.go`。
3. 将执行过程中的 `printProgress` 改为调用共享 renderer。
4. 删除 `app.go` 中旧 helper，保留原有输出断言。

## 风险

- 执行进度 checklist 可能依赖 transient `stageRuntime`，迁移时必须保留 live session id 和失败标记。
- `wo status --json` 不能泄漏 human-only 并行摘要。
- watch 输出的 running marker 和 spinner 不应被普通 status 输出污染。

## 验证策略

合同测试检查展示 helper 已离开 `app.go`，并运行现有 status/view/watch/runner JSON Go 回归。执行阶段可以补充更细的对比测试，但不得弱化当前合同。
