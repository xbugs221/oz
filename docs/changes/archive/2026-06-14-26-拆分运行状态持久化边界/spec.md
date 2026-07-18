# 规格：拆分运行状态持久化边界

## 验收矩阵

| 场景 | 测试 | 证据 | 风险 |
| --- | --- | --- | --- |
| 运行状态边界已拆分且核心行为不变 | `state-runtime-boundary-contract` | `state-runtime-boundary-log` | 只覆盖关键回归入口，不穷尽所有历史 shell 合同 |

### 需求：运行状态边界拆分

系统必须把 sealed run 的状态持久化、prompt 上下文和 git 人工干预守卫从 `state.go` 拆出到独立文件，同时保持原有运行行为。

#### 场景：运行状态边界已拆分且核心行为不变

- **测试文件**：`docs/changes/archive/2026-06-14-26-拆分运行状态持久化边界/tests/test_state_runtime_boundary_contract.sh`
- **真实数据来源**：仓库当前 `internal/app` 源码、现有 Go 单测构造的真实 `State`、临时 git repo 和 run state。
- **入口路径**：从仓库根目录运行 shell 合同测试，测试内部执行 `go test ./internal/app` 的状态机和人工干预相关回归。
- **关键断言**：
  - `internal/app/state_store.go`、`internal/app/run_lock.go`、`internal/app/prompt_context.go`、`internal/app/git_guard.go` 必须存在。
  - `internal/app/state.go` 不再直接定义 state store、prompt context、git guard 和 run lock 的核心 helper。
  - 现有状态机、Go DAG、人工干预和 acceptance preflight Go 回归必须通过。
- **剩余风险**：合同测试不检查每个 helper 的具体文件名以外的完整实现细节，执行阶段仍需 code review 确认没有为了过测试留下重复实现。
