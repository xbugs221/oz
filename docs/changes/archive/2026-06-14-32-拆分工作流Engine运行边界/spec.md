# 规格：拆分工作流 Engine 运行边界

## 验收矩阵

| 需求 | 场景 | required_tests | required_evidence | 关键断言 |
| --- | --- | --- | --- | --- |
| Engine 运行边界拆分 | 状态模型、恢复、运行循环、阶段执行和进度持久化被分离 | engine-boundary | engine-boundary-log | 目标文件存在，核心函数落在对应文件，workflow Go 回归仍通过 |

### 需求：Engine 运行边界拆分

工作流 Engine 必须按持久化模型、运行入口、恢复逻辑、阶段执行和进度持久化拆分文件，避免 `state.go` 继续混合所有核心职责。

#### 场景：状态模型、恢复、运行循环、阶段执行和进度持久化被分离

- 测试文件：`docs/changes/archive/2026-06-14-32-拆分工作流Engine运行边界/tests/engine_boundary_test.sh`
- 真实数据来源：仓库当前 `internal/app` 生产代码、现有 workflow Go 回归测试。
- 入口路径：执行 shell 契约测试，内部检查目标 Go 文件和运行 `go test ./internal/app` 的 workflow 相关测试。
- 关键断言：`State` 模型、`resumeRun`、`runLoop`、`runStage`、`stageProgressWriter` 必须分别落在对应文件；`state.go` 不得继续承载 1000 行级别的混合职责；现有 workflow 回归必须通过。
- 剩余风险：该测试不证明所有 shell 端到端合同都已覆盖，执行阶段仍需按影响面补跑根目录业务测试。
