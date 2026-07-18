# 任务

## 1. 契约测试

- [x] 1.1 运行 `bash docs/changes/archive/2026-06-10-13-修正-wo-status-多轮并行状态展示/tests/test_status_multiround_parallel_display_contract.sh`，确认当前实现失败于目标展示行为缺失，而不是测试语法、路径或环境错误。
- [x] 1.2 保存失败输出到 `test-results/13-status-multiround-parallel-display/status-multiround-parallel-display.log`。

## 2. 修正 human status/watch 聚合

- [x] 2.1 移除 human status/watch 中的 `parallel_group` 和 `parallel_member` 渲染，不再输出 `- 并行 <group>` 和 raw member status。
- [x] 2.2 保留 JSON observability 里的机器可读 artifact 路径，不删除 runner JSON 顶层字段。
- [x] 2.3 execution 起跑且 `planning_context_fanin` 成功时，将规划行 marker 渲染为 `✓`，并隐藏规划 subagent 明细。

## 3. 修正多轮阶段展示

- [x] 3.1 为 review/qa/fix compact 行计算当前代表轮次，优先使用当前 `state.Stage`，否则使用已到达最后一轮。
- [x] 3.2 多轮 marker 保留历史轮次状态，例如 `review_1`、`review_2` completed 且 `review_3` failed 时显示 `✓✓x`。
- [x] 3.3 `statusSubagentRows` 使用当前代表轮次，不再固定读取 `review_1` 或 `qa_1`。

## 4. 修正 subagent 节点匹配

- [x] 4.1 review helper 只匹配 `before_review_<iteration>_<index>`，qa helper 只匹配 `before_qa_<iteration>_<index>`。
- [x] 4.2 implementation helper 继续匹配 `before_execution_<index>`；planning helper 继续匹配 `planning_context_<index>`。
- [x] 4.3 不允许 `review_3`、`qa_2`、`fix_1` 这类 main_stage node 被当成 subagent node。

## 5. 回归验证

- [x] 5.1 重新运行 `bash docs/changes/archive/2026-06-10-13-修正-wo-status-多轮并行状态展示/tests/test_status_multiround_parallel_display_contract.sh`，确认通过。
- [x] 5.2 运行 `go test ./internal/app`，按新意图更新旧 status/watch 断言，不删除业务覆盖。
- [x] 5.3 检查 `wo status` 和 `wo watch` 的现有 compact 输出合同仍保持四列结构、短名 subagent 和 batch 层级。

## 执行记录

- 历史测试同步原因：`internal/app/status_view_test.go` 原先要求失败轮 compact marker 只显示 `x`，与本提案“保留历史轮次状态”的新意图冲突，已更新为验证 `✓x`。
- 历史 compact shell 合同同步原因：`tests/specs/codex-workflow-cli/test_status_watch_compact_output_contract.sh` 原先要求 human 输出展示 `- 并行 implementation_context` 和 raw member status，已按本提案改为禁止 fan-in 泄漏，同时继续断言短名 subagent、四列结构和 batch 层级。
