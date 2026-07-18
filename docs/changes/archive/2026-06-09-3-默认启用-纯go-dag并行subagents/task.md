# 任务

## 1. 契约测试基线

- [x] 1.1 运行 `bash docs/changes/archive/2026-06-09-3-默认启用-纯go-dag并行subagents/tests/test_default_go_dag_run_contract.sh`，确认当前实现因默认 engine/state/status 仍未改为 `go-dag` 而失败。
- [x] 1.2 运行 `bash docs/changes/archive/2026-06-09-3-默认启用-纯go-dag并行subagents/tests/test_go_dag_graph_status_contract.sh`，确认当前实现因默认配置未启用 parallel 且 graph/status 缺少新语义而失败。

## 2. 引入纯 Go DAG engine

- [x] 2.1 添加纯 Go DAG 执行层，候选库为 `github.com/Azure/go-workflow`。
- [x] 2.2 用现有 `WorkflowSpec` 构建 DAG，避免 graph 定义和执行定义分叉。
- [x] 2.3 为每个 DAG node 添加 before/after/failure hook，写入 `state.json`。
- [x] 2.4 默认 `wo run --change <change> --json` 走 `go-dag` engine，不调用 Dagu CLI。

## 3. 默认启用 parallel subagents

- [x] 3.1 将默认 `workflow.parallel.enabled` 改为 `true`。
- [x] 3.2 更新 `wo config` 默认输出，写明 `engine: go-dag` 和 `parallel.enabled: true`。
- [x] 3.3 把 planning_context、implementation_context、review、qa 成员映射为 DAG 并行节点。
- [x] 3.4 fan-in 节点继续产出既有 `parallel-planning-context.json`、`parallel-implementation-context.json`、`parallel-review-N.json` 和 `parallel-qa-N.json`。

## 4. 强化 status 和 graph

- [x] 4.1 `wo status -wN` 人类输出展示 `引擎 go-dag`。
- [x] 4.2 `wo status` 展示总进度、主阶段进度、并行 group 摘要和成员明细。
- [x] 4.3 `wo graph --format mermaid` 默认展示 parallel fan-out/fan-in 与主阶段 gate。
- [x] 4.4 `wo status --run-id <run-id> --json` 保持现有 runner contract，不新增 parallel/members 字段。

## 5. 历史路径降级

- [x] 5.1 将旧 Go 状态机标记为 legacy/historical backup。
- [x] 5.2 默认 help 和文档不再推荐 legacy engine。
- [x] 5.3 Dagu CLI 路径如保留，只作为非默认历史备份或调试入口。

## 6. 验证

- [x] 6.1 运行本提案两个契约测试并确保通过。
- [x] 6.2 运行 `go test ./internal/app`。
- [x] 6.3 运行 `go test ./...`。
- [x] 6.4 运行 `oz validate 3-默认启用-纯go-dag并行subagents --json`。

## 历史测试更新

- [x] 更新仍假定 `parallel.enabled: false` 的旧测试，使其表达新的默认行为。
- [x] 更新仍假定默认 engine 为旧状态机或 Dagu CLI 的旧测试。
- [x] 保留 JSON runner contract 兼容测试，防止 status 机器接口被人类输出污染。

历史测试更新原因：默认 parallel 已改为 true，旧单元测试的 fake runner 需要真实写入 subagent artifact，避免继续表达旧线性状态机路径。
