# 拆分运行状态持久化边界

本提案把 `internal/app/state.go` 中的运行状态持久化、prompt 上下文和 git 变更守卫拆到独立文件，降低 sealed run 状态机后续修改的误伤风险。

交付目标：

- 保持现有 `wo run`、`wo resume`、Go DAG、人工干预检测和 prompt 快照行为不变。
- 将 state JSON 读写、run lock、prompt context、git snapshot guard 从 `state.go` 拆出，形成可审查的业务边界。
- 增加合同测试证明重构真实发生，并用现有 Go 回归覆盖状态机关键路径。

非目标：

- 不改变 `State` JSON 字段、运行目录结构、lock 文件格式或 runner JSON 契约。
- 不重写 Go DAG 调度、不调整阶段决策。

验收入口：

- `bash docs/changes/26-拆分运行状态持久化边界/tests/test_state_runtime_boundary_contract.sh`
- `go test ./internal/app -count=1`

执行阶段默认上下文：先读 `internal/app/state.go`、`internal/app/stage_decision.go`、`internal/app/go_dag.go`、`internal/app/runner_contract.go` 和本提案测试，先让合同测试因目标边界缺失而失败，再做最小拆分。
