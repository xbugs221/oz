# 提案：统一状态展示视图入口

## 背景

仓库已经存在 `status_view.go`，说明状态展示正在向共享 view model 收敛。但 `app.go` 仍保留 `stageChecklistLines`、`visibleSessionItems`、`plannerSessionID`、`sessionRoleID`、耗时汇总等展示计算。这样会导致 human status、watch、runner JSON 和执行进度 checklist 出现多套来源，后续增加并行子代理观测或耗时字段时容易漏改。

## 变更

- 让 `status_view.go` 负责状态展示所需的阶段、session、subagent、artifact、duration、stale run 判定等数据计算。
- 让 `status_render.go` 负责把 `statusView` 渲染为 human/watch 文本。
- 让执行进度 checklist 也复用共享 view model，避免 `app.go` 保留另一套 session 分组逻辑。
- 删除或迁出 `app.go` 中状态展示 helper，只保留交互流程和命令编排。

## 为什么

状态展示是用户排查失败 run、恢复 batch 和理解并行子代理结果的主要入口。共享 view model 可以保证 human 和 JSON 的行为差异是有意的，而不是不同 helper 演化出来的偶然差异。

## 非目标

- 不新增状态字段。
- 不改变已有 JSON 字段名。
- 不调整 Go DAG 节点状态语义。
