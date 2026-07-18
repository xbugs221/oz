# 规格

## 验收矩阵

| 需求 | 场景 | required_tests | required_evidence | 剩余风险 |
| --- | --- | --- | --- | --- |
| 验收硬合同 | 弱验收合同被 `oz validate` 拒绝 | `strict-acceptance-contract-validation` | `strict-acceptance-contract-log` | 弱断言识别只覆盖明确反模式，不能替代 review |
| 执行上下文聚焦硬合同 | execution prompt 默认只读硬合同和用户简报 | `execution-hard-contract-prompt` | `execution-hard-contract-prompt-log` | prompt 仍可能按需提到长文档名称，但不能把它们列为默认必读 |
| 状态暴露合同门槛 | `oz status --json` 暴露 `brief.md` 和验收合同摘要 | `status-hard-contract-summary` | `status-hard-contract-summary-log` | status 只暴露合同规模，不负责证明测试已经通过 |

### 需求：验收硬合同

系统必须在提案进入执行前拒绝弱验收合同，确保执行智能体拿到的 `acceptance.json` 已经绑定真实测试、业务级断言和覆盖矩阵。

#### 场景：弱验收合同被 `oz validate` 拒绝

- **给定** 一个 active change 包含 `brief.md`、现有提案文件、`acceptance.json` 和 `docs/changes/archive/2026-06-10-8-强化验收硬合同并精简执行上下文/tests/`
- **且** `required_tests[]` 缺少业务级 `assertions`
- **当** 用户运行 `oz validate <change> --json`
- **则** 命令失败
- **且** 输出说明失败原因与 `assertions` 或弱验收合同有关
- **并且** 当 `assertions` 只包含 `HTTP 200` 这类弱表面信号时也必须失败
- **并且** 当合同包含业务级断言、真实测试路径、覆盖矩阵和证据引用时必须通过

对应测试：`docs/changes/archive/2026-06-10-8-强化验收硬合同并精简执行上下文/tests/test_strict_acceptance_contract_validation.sh`。真实数据来源是脚本创建的临时 oz 项目；入口路径是编译后的真实 `oz validate` 命令；关键断言是弱合同失败、强合同通过、错误原因指向验收合同质量。剩余风险是弱断言识别只能覆盖明确反模式。

### 需求：执行上下文聚焦硬合同

execution 阶段必须默认聚焦硬合同和用户简报，避免把所有长文档作为同等权重上下文输入。

#### 场景：execution prompt 默认只读硬合同和用户简报

- **给定** 系统渲染内置 `wo-start.md` execution prompt
- **当** prompt 发给执行智能体
- **则** prompt 必须要求默认读取 `brief.md`、`acceptance.json` 和 `docs/changes/archive/2026-06-10-8-强化验收硬合同并精简执行上下文/tests/`
- **并且** prompt 必须要求先运行 `acceptance.json.required_tests[].command`
- **并且** prompt 不得继续使用“默认读取 `proposal.md`、`design.md`、`spec.md`、`task.md`、`acceptance.json` 和 `docs/changes/archive/2026-06-10-8-强化验收硬合同并精简执行上下文/tests/`”的全量文档合同

对应测试：`docs/changes/archive/2026-06-10-8-强化验收硬合同并精简执行上下文/tests/test_execution_prompt_hard_contract_focus.sh`。真实数据来源是内置 prompt 模板；入口路径是 `go test ./internal/app` 渲染真实模板；关键断言是 prompt 包含硬合同入口且不再包含旧的全量必读句式。剩余风险是 prompt 仍可在按需规则中提到长文档名称。

### 需求：状态暴露合同门槛

系统必须让调度器和用户从 `oz status --json` 中看到硬合同是否存在及其规模，而不是只看到 task checkbox。

#### 场景：`oz status --json` 暴露 `brief.md` 和验收合同摘要

- **给定** 一个 active change 包含 `brief.md`、提案文件、`acceptance.json`、`docs/changes/archive/2026-06-10-8-强化验收硬合同并精简执行上下文/tests/` 和已完成任务
- **当** 用户运行 `oz status <change> --json`
- **则** `artifacts` 必须包含 `brief.md`
- **并且** `acceptance.required_tests.total` 等于 `acceptance.json` 中的 required tests 数量
- **并且** `acceptance.required_evidence.total` 等于 `acceptance.json` 中的 required evidence 数量
- **并且** `acceptance.coverage.total` 等于 `acceptance.json` 中的 coverage 数量

对应测试：`docs/changes/archive/2026-06-10-8-强化验收硬合同并精简执行上下文/tests/test_status_hard_contract_summary.sh`。真实数据来源是脚本创建的临时 oz 项目；入口路径是编译后的真实 `oz status` 命令；关键断言是 JSON artifact 和验收摘要字段存在且数量正确。剩余风险是 status 不负责执行测试，只报告合同结构。
