<!-- 文件目的：拆分 19 提案的实现步骤、验证条件和执行顺序。 -->

# 任务

## 0. 先运行创建阶段契约测试

- [x] 运行 `bash docs/changes/archive/2026-06-12-19-拆分wo门禁状态并新增验收预检阻断/tests/test_gate_state_and_acceptance_preflight_contract.sh`
- [x] 确认初始失败来自目标行为缺失：`State.ArtifactGates`、`State.AcceptancePreflight`、`blocked_acceptance_contract` 或 `runAcceptancePreflight` 尚未实现，而不是测试语法、路径或环境错误

## 1. 拆分门禁状态

- [x] 新增 artifact gate 独立持久化字段
- [x] 让 `recordStageArtifactGateFailure()` 写入 `artifact_gates`
- [x] artifact gate 修复通过后清理当前 stage 的 artifact gate `last_error`
- [x] 保留旧 `state.validation` 兼容读取，但新 run 不再把 artifact gate failure 写入 validation

## 2. acceptance preflight 阻断

- [x] 新增 `blocked_acceptance_contract` 终态
- [x] 新增 `acceptance_preflight` 状态字段
- [x] 在 execution artifact gate 通过后运行 preflight
- [x] evidence 无 producer、live evidence 无可复核入口或 evidence 覆盖链不完整时进入 `blocked_acceptance_contract`
- [x] preflight 失败不得进入 review、QA、fix 或 archive

## 3. 状态展示和兼容

- [x] 更新 human/json status 输出，区分 artifact gate、validation gate 和 acceptance preflight
- [x] 旧 run 的 `validation.kind=artifact` 继续能被用户看懂
- [x] batch 状态识别新的 `blocked_acceptance_contract`

## 4. 根测试和文档

- [x] 将创建阶段合同沉淀为根测试或更新现有 app Go 测试
- [x] 更新 `docs/specs/codex-workflow-cli/spec.md`
- [x] 运行 `go test ./...`
- [x] 运行 `oz validate 19-拆分wo门禁状态并新增验收预检阻断 --json`
