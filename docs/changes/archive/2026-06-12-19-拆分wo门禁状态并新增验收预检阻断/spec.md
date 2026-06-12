<!-- 文件目的：定义 19 提案的验收行为、测试矩阵和剩余风险。 -->

# 规格

## 验收矩阵

| 需求 | 场景 | 测试 | 证据 | 风险 |
| --- | --- | --- | --- | --- |
| artifact gate 与 validation gate 状态分离 | artifact gate failure 不再写入 command validation 状态 | `gate-state-preflight-contract` | `gate-state-preflight-log` | 旧 run 的 `validation.kind=artifact` 只要求兼容展示，不要求迁移 |
| execution 后执行 acceptance preflight | evidence 无 producer 时阻断为验收合同问题 | `gate-state-preflight-contract` | `gate-state-preflight-log` | producer 追溯第一版是保守启发式，不证明 evidence 一定被真实生成 |

### 需求：artifact gate 与 validation gate 状态分离

wo 必须用独立状态记录阶段产物门禁，使 `state.validation` 在新 run 中只表示确定性命令
validation gate。

#### 场景：artifact gate failure 不再写入 command validation 状态

- **对应测试**：`docs/changes/19-拆分wo门禁状态并新增验收预检阻断/tests/test_gate_state_and_acceptance_preflight_contract.sh`
- **真实数据来源**：真实 `State`、真实 `recordStageArtifactGateFailure()`、真实 JSON marshal
- **入口路径**：`go test ./internal/app -run 'TestArtifactGateFailureUsesArtifactGateState|TestAcceptancePreflight'`
- **关键断言**：
  - artifact gate failure 写入 `state.artifact_gates[stage]`
  - `state.validation[stage]` 不再出现 `kind=artifact`
  - JSON 中存在 `artifact_gates`，不存在由 artifact gate 产生的 `validation.execution`
- **剩余风险**：该场景不测试 status UI 展示；执行阶段需补根回归或更新现有 status 测试。

### 需求：execution 后执行 acceptance preflight

wo 必须在 execution artifact gate 通过后、进入 review/QA/fix/archive 前检查 acceptance 合同是否
可执行。第一版发现合同不可执行时必须阻断，让用户检查验收合同。

#### 场景：evidence 无 producer 时阻断为验收合同问题

- **对应测试**：`docs/changes/19-拆分wo门禁状态并新增验收预检阻断/tests/test_gate_state_and_acceptance_preflight_contract.sh`
- **真实数据来源**：真实 `ReadAcceptance()`、真实 `Acceptance` 结构、真实 `State` 阻断状态
- **入口路径**：`go test ./internal/app -run 'TestArtifactGateFailureUsesArtifactGateState|TestAcceptancePreflight'`
- **关键断言**：
  - evidence 没有任何 required test 命令、路径、用途或断言可追溯 producer 时，preflight 返回失败
  - run 状态变为 `blocked_acceptance_contract`
  - preflight 失败写入 `state.acceptance_preflight`，不写入 `state.validation.execution`
  - evidence 可从 required test 追溯 producer 时，preflight 通过且 run 保持 running
- **剩余风险**：该场景不执行真实 browser/live 环境，只验证合同在进入 review 前能被阻断。
