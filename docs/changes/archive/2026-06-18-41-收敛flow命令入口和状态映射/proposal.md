# 提案：收敛 flow 命令入口和状态映射

## 背景

当前 `internal/app` 已经经历过多轮拆分，但仍存在三类高价值重构点：

- `Run`、`commandNeedsWorkflowTools` 和 repo command dispatch 对同一命令面的职责认知分散，`config` 等命令路径存在重复入口。
- workflow 阶段、并行组、display group、artifact 命名分别散落在 graph、config、status view 和 status parallel 逻辑中。
- 新的 status view model 已经存在，但交互进度和部分 legacy checklist 仍绕过统一模型。

这些问题不会马上表现为单个 bug，但会让后续 DAG、验收执行、batch/restart 等重构反复触碰同一批字符串和分支，增加回归风险。

## 变更目标

1. 建立 `oz flow` 命令 registry。
   - 每个命令声明是否需要 git repo、是否需要 workflow backend 预检，以及对应 handler。
   - `Run` 只负责启动流程和调用 registry，不再维护第二套命令分类。

2. 建立 workflow topology 边界。
   - 阶段种类、迭代阶段、并行组、display anchor、artifact 名称映射集中在一个生产代码边界。
   - `graph.go`、`config_parallel.go`、`status_view_model.go` 和 `status_parallel.go` 只消费 topology，不再复制硬编码映射。

3. 统一 status view 入口。
   - status、watch、batch status 和 engine progress 都从 `statusView` 或其适配器生成用户可见行。
   - legacy checklist 逻辑不得继续作为公共展示路径的事实源。

## 成功标准

- 契约测试能证明结构边界已收敛，并且真实 Go CLI 回归通过。
- `oz flow status/watch/run/config` 等既有命令行为保持稳定。
- 后续要拆 Go DAG 节点执行、acceptance runner 或 batch/restart 时，不需要继续在多个文件同步维护同一套阶段和并行组映射。
