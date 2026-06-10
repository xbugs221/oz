# 简报

本次变更解决 `wo status` 和 `wo watch` 的 human 输出过度展示顶部运行编号，以及 `watch` 在终端刷新时可能残留旧首行的问题。

交付目标：

- `wo status` 和 `wo watch` 默认直接从变更提案列表开始显示。
- batch header、workflow header 和默认“正在查看最近一次批量工作流”提示不再占据顶部。
- `wo status` 的 running 阶段仍使用静态 `→`。
- `wo watch` 的 spinner 只出现在正在执行的阶段行，不出现在顶部。
- `wo watch` 在窄终端、长中文提案名换行后连续刷新，不再残留旧帧首行。

非目标：

- 不改变 `--json` 输出。
- 不改变 `-bN`、`-wN` 的解析规则。
- 不改变 batch/run 持久化状态格式。
- 不重新设计工作流阶段状态机。

验收入口：

- `bash docs/changes/9-精简-wo-status-watch-提案列表并修正watch清屏/tests/test_status_watch_proposal_list_contract.sh`
- `bash docs/changes/9-精简-wo-status-watch-提案列表并修正watch清屏/tests/test_watch_tty_clear_long_change_contract.sh`

执行阶段默认上下文：

先读 `internal/app/status_view.go`、`internal/app/batch.go`、`internal/app/app.go` 中 status/watch human 渲染路径。核心实现应收敛到共享的提案列表渲染逻辑，让 `status` 与 `watch` 只在 running marker 上不同。
