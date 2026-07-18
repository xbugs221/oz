# 规格：拆分子智能体执行边界

## 验收矩阵

| 需求 | 场景 | required_tests | required_evidence | 关键断言 |
| --- | --- | --- | --- | --- |
| 子智能体执行边界拆分 | retry、只读边界、artifact 和 prompt 职责被分离 | subagent-boundary | subagent-boundary-log | 目标文件存在，关键函数落在对应文件，subagent Go 回归仍通过 |

### 需求：子智能体执行边界拆分

subagent 执行必须把重试执行、只读边界、artifact 处理和 prompt 组装拆成独立文件，避免单一文件继续混合并行 helper 的全部关键逻辑。

#### 场景：retry、只读边界、artifact 和 prompt 职责被分离

- 测试文件：`docs/changes/archive/2026-06-15-33-拆分子智能体执行边界/tests/subagent_boundary_test.sh`
- 真实数据来源：仓库当前 `internal/app` 生产代码和现有 subagent/parallel Go 回归测试。
- 入口路径：执行 shell 契约测试，内部检查目标 Go 文件和运行 `go test ./internal/app` 的 subagent 相关测试。
- 关键断言：入口文件变薄；attempt、boundary、artifact、prompt 分别有独立文件；子智能体超时、只读边界、artifact 兜底、并发 artifact 写入相关 Go 回归必须通过。
- 剩余风险：该测试不覆盖外部真实 Codex/Pi CLI 调用，需要执行阶段按环境补充手工或 shell 业务验证。
