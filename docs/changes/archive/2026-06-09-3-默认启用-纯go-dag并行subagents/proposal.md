# 默认启用纯 Go DAG 并行 subagents

## 背景

`wo` 当前已经有显式 Dagu CLI engine 和 parallel subagents 的雏形，但默认执行仍走旧 Go 状态机，parallel 也默认关闭。这和期望的用户心智不一致：正常运行 `wo` 时，用户应直接获得清晰的 DAG 调度、并行调研和可观察状态，而不是额外安装或显式选择一个外部调度器。

同时，Dagu 作为外部 CLI 和带 Web UI 的项目，会把 `wo` 的默认路径绑到额外二进制和非纯 Go 生态上。`wo` 更适合使用纯 Go DAG 执行库作为内部调度层，并继续保留自己的 `state.json`、artifact、status 和 graph 输出。

## 变更目标

- 默认执行引擎改为内嵌纯 Go DAG engine，候选库为 `github.com/Azure/go-workflow`。
- 默认启用 parallel subagents，让 planning context、implementation context、review 和 QA 的成员真实并行执行并 fan-in。
- 旧 Go 状态机只保留为历史备份路径，不再作为公开推荐 engine。
- `wo status` 输出清晰展示 engine、DAG 节点、并行成员、当前阶段和 artifact 状态。
- `wo graph --format json|mermaid` 继续由 `wo` 自己导出用户可理解的工作流图，不依赖执行库的可视化能力。

## 非目标

- 不引入 Dagu CLI 作为默认运行依赖。
- 不引入 Temporal、外部数据库、后台服务或 Web UI。
- 不在本次变更中移除历史状态机源码，只隐藏或降级为备份路径。
- 不改变 `wo status --run-id <id> --json` 的现有 runner contract 字段。

## 验收重点

- 默认 `wo run --change <change> --json` 不调用 `dagu`，而是使用内嵌 Go DAG engine。
- 默认配置和生成的 `wo.yaml` 表达 `go-dag` engine 和 `parallel.enabled: true`。
- DAG 图中能看到 planning/implementation/review/QA 的并行节点和 fan-in 关系。
- 人类可读 `wo status` 能看到 engine、并行成员进度和主阶段进度。
- JSON runner status 继续保持兼容，不新增 `parallel` 或 `members` 字段。
