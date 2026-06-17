# 任务：收敛 flow 命令入口和状态映射

- [x] 先运行创建阶段契约测试：`bash docs/changes/41-收敛flow命令入口和状态映射/tests/test_flow_boundary_convergence_contract.sh`
  - 预期初始失败点应是目标边界未实现，例如缺少 `flow_command_registry.go`、`workflow_topology.go`，或公共 status/progress 仍直接调用 legacy checklist。
  - 如果失败于 shell 语法、路径不存在、Go 测试环境不可用等非目标原因，先修正测试合同。

- [x] 收敛 `oz flow` 命令入口。
  - 新增命令 registry，表达 repo 需求、workflow tools 需求和 handler。
  - 让 `Run` 和 tools 预检查询 registry，删除 `app.go` 与 `command_dispatch.go` 对同一命令的重复判断。
  - 保持 `oz flow config --global`、`oz flow update`、`oz flow graph`、`oz flow run/status/watch/restart` 等入口行为不变。

- [x] 收敛 workflow topology。
  - 新增 topology helper，集中阶段、并行组、display group、artifact 映射。
  - 改造 graph/config/status 消费 topology，不再各自维护硬编码映射。
  - 保持现有 graph JSON、Mermaid、status 子代理行、parallel config 校验行为不变。

- [x] 统一公共 status view 输出。
  - 让 status、watch、batch status 和 engine progress 走 `statusView` 或薄适配器。
  - 保留必要兼容函数时，确保公共入口不再直接依赖 legacy checklist 作为事实源。

- [x] 验证。
  - 运行 `bash docs/changes/41-收敛flow命令入口和状态映射/tests/test_flow_boundary_convergence_contract.sh`。
  - 运行 `go test ./... -count=1`。
  - 按影响面补跑 status/watch、graph、parallel、command dispatch 相关 shell specs。
  - 运行 `oz validate 41-收敛flow命令入口和状态映射 --json`。
