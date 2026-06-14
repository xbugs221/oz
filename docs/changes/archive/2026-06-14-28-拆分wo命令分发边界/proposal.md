# 提案：拆分 wo 命令分发边界

## 背景

`internal/app/app.go` 的 `Run` 函数承载了早期命令、repo 解析、工具预检、engine 构造、JSON runner 命令、人类命令、交互菜单、规划 session 和状态展示辅助。命令数量持续增加后，大 switch 变成主要变更热点。

## 变更

- 新增 `command_dispatch.go`，承载 repo 相关命令分发和参数校验。
- 新增 `interactive.go`，承载交互菜单、未完成 run/batch 选择和 change 选择流程。
- 新增 `planning.go`，承载 planning prompt、planning session 文件和规划命令启动。
- `app.go` 保留 `Run` 的最小装配逻辑，以及少量通用入口。

## 为什么

命令分发和交互流程变化频率高，但它们不应和状态机、状态展示、配置读取混在同一个文件。拆开后，新增命令只需要审查分发文件和对应 handler，交互改动也不会影响 JSON runner API。

## 非目标

- 不引入第三方 CLI 框架。
- 不重命名用户命令。
- 不修改 `oz` CLI。
