# 设计：拆分 wo 命令分发边界

## 目标结构

- `app.go`：`Run` 入口、早期无需 repo 命令、repo/context/engine 装配。
- `command_dispatch.go`：repo 相关命令的 dispatch 和 handler，例如 run/resume/batch/restart/status/abort/clean/watch。
- `interactive.go`：无参数启动时的人类交互流程、选择 active change、处理未完成 run/batch。
- `planning.go`：规划 prompt、规划工具调用和 planning session id 捕获。

## 关键取舍

不引入新的命令框架。当前命令面较小，表驱动或小 handler 已足够；引入框架会扩大依赖和命令文案变化风险。

## 风险

- 早期命令中有些不需要 git repo，例如 `--version`、`--help`、`update`、全局 `config`。拆分时必须保留这些命令在非 git 目录可用。
- JSON runner 命令的错误路径会写失败状态，不能因为 handler 拆分而漏写。
- 人类交互流程依赖 `os.Stdin` 和 planning session 文件，迁移时不能破坏 session 捕获。

## 验证策略

合同测试要求 command/interactive/planning 边界文件存在，并检查 `app.go` 不再直接包含核心 repo 命令 case。随后运行 `internal/app` 和 `cmd/oz` 全量 Go 单测，覆盖真实命令面和独立 `oz` CLI。
