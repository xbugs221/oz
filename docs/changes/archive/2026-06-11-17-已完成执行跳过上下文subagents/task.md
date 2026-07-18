# 任务

## 1. 契约测试

- [x] 1.1 先运行 `bash docs/changes/archive/2026-06-11-17-已完成执行跳过上下文subagents/tests/test_skip_execution_context_when_tasks_done.sh`，确认当前实现会因为已完成 task 仍启动 subagent 而失败。
- [x] 1.2 先运行 `bash docs/changes/archive/2026-06-11-17-已完成执行跳过上下文subagents/tests/test_run_execution_context_when_tasks_pending.sh`，确认未完成 task 的既有路径有明确保护。

## 2. 实现

- [x] 2.1 在 go-dag execution 前置调度中加入 task 完成度判断，复用 `ChangeTasksDone`。
- [x] 2.2 当 task 已全部完成时，跳过 execution 前 advisory subagents、fan-in gate 和 execution 主 agent，并推进到后续阶段。
- [x] 2.3 当 task 未完成时，保持现有 subagent fan-out、fan-in、execution artifact gate 行为。
- [x] 2.4 确保 status/watch 不把被跳过的 execution context subagents 展示成已运行会话。

## 3. 验证

- [x] 3.1 重新运行本提案 `docs/changes/archive/2026-06-11-17-已完成执行跳过上下文subagents/tests/` 下两个契约测试。
- [x] 3.2 运行相关根目录回归测试，至少覆盖 go-dag、parallel context、status 显示和 stage artifact gate。
- [x] 3.3 记录 `test-results/17-skip-execution-context/` 下的 runtime log 和 state snapshot，供 QA 复核。

备注：1.1、1.2、3.1 已执行；当前 shell 契约测试在进入目标断言前被固定 `agy` CLI 预检阻塞，未产生成功路径 state snapshot。已补充 `internal/app` 回归测试覆盖同等 go-dag 行为，并保留 runtime log 记录阻塞原因。
