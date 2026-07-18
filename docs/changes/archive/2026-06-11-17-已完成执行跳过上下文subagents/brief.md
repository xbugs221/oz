# 已完成执行跳过上下文 subagents

用户在 `wo` 状态中看到执行阶段已经不需要实际执行，但执行前的代码侦察和外部资料 subagents 仍被启动，造成资源浪费和状态噪音。本次变更要让 `wo` 在进入执行前先读取 `oz status` 的 task 完成度：如果当前 active change 的任务已经全部完成，就直接跳过服务于执行阶段的 advisory subagents 和执行主 agent，继续后续验收与归档；如果任务未完成，仍保持现有 subagents 辅助执行的路径。

本次不关闭 review/QA 的 gate input subagents，也不把 task 全部完成等同于整条工作流完成。后续 review、QA、archive 仍按既有合同运行。

验收入口：

- `docs/changes/archive/2026-06-11-17-已完成执行跳过上下文subagents/tests/test_skip_execution_context_when_tasks_done.sh`
- `docs/changes/archive/2026-06-11-17-已完成执行跳过上下文subagents/tests/test_run_execution_context_when_tasks_pending.sh`

