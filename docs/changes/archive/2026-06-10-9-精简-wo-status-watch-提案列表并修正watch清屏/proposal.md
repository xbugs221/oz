# 精简 wo status/watch 提案列表并修正 watch 清屏

## 问题

当前 `wo status/watch` 的 human 输出会先展示运行编号，再展示具体提案：

```text
| b1 1/1
- 198-严格验证Kestra真实UI嵌入并补齐运维复现文档
  → w1
  规划阶段 - - -
```

用户观察任务进度时，真正关心的是提案和阶段进度。顶部的 batch/workflow 编号只在显式定位任务时有用，不应成为默认输出的第一视觉焦点。

同时，`wo watch` 当前按逻辑行数回退并清屏。长中文提案名在窄终端自动换行后，实际屏幕行数大于逻辑行数，下一帧可能清不掉旧帧顶部，导致首行重复残留。

## 目标

- 默认 `wo status` 直接显示变更提案列表。
- 默认 `wo watch` 直接显示变更提案列表。
- 单 workflow 和 batch 都使用同一种提案列表结构。
- `wo status` 的 running marker 保持静态 `→`。
- `wo watch` 的 spinner 只替换 running 阶段行的 marker。
- TTY watch 刷新必须按真实屏幕占用清理，或使用不会受换行影响的整屏刷新策略。

## 范围

- 调整 `internal/app/status_view.go` 的 human compact 行输出职责，避免 workflow header 混入阶段列表。
- 调整 `internal/app/batch.go` 和 `internal/app/app.go` 的 batch/watch human 渲染。
- 调整 `wo status` 默认 batch 提示，避免顶部提示挡住提案列表。
- 调整 `wo watch` TTY 刷新策略，修复长行换行后的残留。
- 更新旧测试中关于 header spinner 的断言。

## 非目标

- 不改变 `wo status --run-id <run-id> --json`。
- 不改变 runtime state JSON。
- 不改变 `ResolveStatusTarget`、`resolveWatchTarget` 的别名解析。
- 不引入新的终端 UI 框架。

## 验收

本提案包含两个真实测试入口：

- `test_status_watch_proposal_list_contract.sh`：用真实 repo runtime state 覆盖 `status/watch` 渲染函数，验证输出直接从提案列表开始，且 watch spinner 在 running 阶段行。
- `test_watch_tty_clear_long_change_contract.sh`：构造真实 `wo` 二进制和伪 TTY，验证长中文提案名在窄终端连续刷新后没有旧首行残留。
