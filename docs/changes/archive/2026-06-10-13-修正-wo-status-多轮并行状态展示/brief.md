# 简述：修正 wo status 多轮并行状态展示

`wo status` 在 go-dag 并行 subagent 和多轮 review/fix 交错时会输出误导信息：规划阶段已经完成却显示 `-` 并展开规划 subagent；execution/review 下出现 `- 并行 implementation_context/review ... failed` 和 raw member status；第三轮 review 失败时，审核行只显示 `x`，并且子代理明细错误地混入第一轮 session 或撞上主 review 节点。

## 交付目标

- human `wo status` / `wo watch` 保持极简阶段视图，不再输出 parallel fan-in summary 行和 fan-in member raw status。
- 已从 execution 起跑的 run 必须把 planning_context fan-in 成功视为规划阶段已完成，显示 `规划阶段 - ✓ -`，且不展示规划 subagent 明细。
- 多轮 review/qa/fix 的 compact 阶段 marker 必须保留历史轮次，例如两轮 review 通过、第三轮 review 失败时显示 `✓✓x`。
- review/qa 子代理明细必须绑定当前实际轮次，不能固定读取第一轮，也不能把主阶段节点误判为某个 subagent 节点。

## 非目标

- 不改变 go-dag 调度顺序、parallel artifact schema、subagent 执行策略或 review/fix/qa 状态机。
- 不修复历史 run 的业务失败原因，只修复状态展示和观测聚合。
- 不删除 JSON observability 中供工具使用的 artifact 路径；本提案只约束 human status/watch 的极简输出。

## 验收入口

- `bash docs/changes/13-修正-wo-status-多轮并行状态展示/tests/test_status_multiround_parallel_display_contract.sh`
- `go test ./internal/app`

## 执行阶段默认上下文

优先阅读：

- `internal/app/status_view.go`
- `internal/app/status_parallel.go`
- `internal/app/graph.go`
- `internal/app/go_dag.go`
- `internal/app/node.go`
- `internal/app/parallel.go`
- `internal/app/status_view_test.go`
- `tests/specs/codex-workflow-cli/test_status_watch_compact_output_contract.sh`
- `tests/specs/codex-workflow-cli/test_status_parallel_summary_contract.sh`

