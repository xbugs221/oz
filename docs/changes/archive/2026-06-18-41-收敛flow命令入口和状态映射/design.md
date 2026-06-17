# 设计：收敛 flow 命令入口和状态映射

## 决策 1：命令 registry 只承载路由元数据和 handler

新增命令 registry 边界，推荐文件名为 `internal/app/flow_command_registry.go`。registry 至少表达：

- 命令名和别名。
- 是否需要 git repo。
- 是否需要 workflow tools 预检。
- human/JSON handler 的调用入口。

`Run` 应保留进程级启动逻辑，例如 version/help、signal context、engine 初始化，但不再通过独立 switch 维护 repo 命令和 tools 预检规则。

取舍：不引入第三方 CLI 框架，避免为了结构收敛扩大依赖和迁移成本。

## 决策 2：workflow topology 是阶段和并行组映射的生产边界

新增 topology 边界，推荐文件名为 `internal/app/workflow_topology.go`。它应覆盖：

- 阶段 kind、可迭代阶段和 display stage。
- 并行组配置名和 graph display group 的转换。
- 每个阶段可挂载的 before group。
- group artifact 和 member artifact 的命名输入。

`graph.go`、`config_parallel.go`、`status_view_model.go`、`status_parallel.go` 只能调用 topology helper，不能继续各自写 `planning_context`、`implementation_context`、`before_review`、`before_qa`、`before_execution` 的映射分支。

取舍：先集中映射，不强制把所有阶段状态 enum 改成新类型，避免一次性重写 scheduler。

## 决策 3：status view 统一公共展示入口

公共 status 输出包括：

- `oz flow status`
- `oz flow watch`
- batch status 行
- engine progress 刷新

这些入口必须通过 `statusView` 或一个薄适配器生成展示行。legacy `stageChecklistLines` 可以暂时保留给过渡测试或兼容内部函数，但不得被公共展示入口直接调用。

## 风险

- 命令 registry 可能误分类无仓库命令，导致 `oz flow config --global`、`oz flow update` 等命令在非 git 仓库中失败。契约测试要求真实 Go 命令面回归覆盖该风险。
- topology 收敛如果漏掉 display group 映射，会影响 DAG graph 和 status 子代理显示。契约测试会禁止关键文件继续使用重复硬编码，执行阶段还应补跑相关 status/graph Go 测试。
- status view 统一可能改变进度输出文案或行顺序。执行阶段必须以现有 status/watch 回归为准，必要时保留适配器而不是改用户可见合同。
