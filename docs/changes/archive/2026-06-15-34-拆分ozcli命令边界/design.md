# 设计：拆分 ozcli 命令边界

## 技术决策

1. 保持 `internal/ozcli` 单包，不引入 cobra 等框架。
2. `cli.go` 放置 `Main`、`cli.run`、顶层 help 和通用 CLI 结构。
3. `cmd_install.go` 放置 install 命令和 install help。
4. `cmd_change.go` 放置 list/create/status、状态 payload、任务进度和 change 编号解析。
5. `cmd_validate.go` 放置 validate 命令、`validateChange`、acceptance 文件绑定和 runtime artifact policy。
6. `cmd_archive.go` 放置 archive 命令、任务完成检查和归档文件移动。
7. `version.go` 或保留的小文件放置版本解析和 source root 定位。

## 取舍

不做命令框架迁移，只用文件边界表达职责，避免为小 CLI 引入额外依赖。

## 风险

- 拆分时可能遗漏共享 helper 的测试覆盖。
- validate/archive 对文件路径和错误文案敏感，必须保留现有测试断言。
