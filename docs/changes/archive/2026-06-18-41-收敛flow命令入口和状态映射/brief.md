# 简报：收敛 flow 命令入口和状态映射

本次提案要解决 `oz flow` 后续重构的三个高价值阻塞点：命令入口仍由多个 switch 分散维护，阶段/并行组/status 映射在 graph、config、status 多处重复，交互进度仍保留 legacy checklist 路径。目标是在不改变 CLI 用户行为的前提下，建立命令 registry、workflow topology 和统一 status view 边界。

交付目标：

- `oz flow` 命令的 repo 需求、workflow tools 需求和执行函数从单一 registry 查询，避免 `app.go` 与 `command_dispatch.go` 重复判断。
- workflow 阶段、并行组、显示锚点和 artifact 命名由一个 topology 边界提供，graph/config/status 不再各自硬编码同一组字符串。
- status、watch、batch 和进度刷新都通过同一 status view 模型构造用户可见行，legacy checklist 仅作为兼容内部实现或被移除。
- 现有 Go 命令面回归必须继续通过。

非目标：

- 不重写 Go DAG 调度算法。
- 不拆 acceptance runner 和 update installer。
- 不改变 `oz flow` 现有用户文案、命令参数、状态 JSON 字段含义。

验收入口：

- `bash docs/changes/41-收敛flow命令入口和状态映射/tests/test_flow_boundary_convergence_contract.sh`
- `oz validate 41-收敛flow命令入口和状态映射 --json`

执行阶段默认上下文：先读 `spec.md` 的验收矩阵和契约测试，再按 `task.md` 从测试失败开始实施。实现时优先移动/收敛边界，不做无关格式化和大规模命名 churn。
