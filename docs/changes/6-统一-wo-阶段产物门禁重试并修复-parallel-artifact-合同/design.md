# 设计

## 当前问题边界

当前默认路径是：

```text
go-dag nodeRunStage
  -> runStage 调用当前角色 agent
  -> nodeStageDone/artifactDone 检查产物
  -> err: 部分 schema 错误会记录 Stage artifact gate failed
  -> done=false: 直接 failNodeState
```

这导致 `execution` task 未完成、`fix` summary 缺失、`archive` delivery summary 缺失这类 `done=false` 场景不会进入同阶段修正，而是直接让 run/batch failed。parallel subagent 也有类似裂缝：字段类型错误会在 subagent 局部 retry，但 `severity: "info"` 这种语义错误直到 `validateMemberResult` 才失败，并且没有 resume 同一 subagent 修正。

另一个同源问题是提示词精简边界过宽：首轮 execution/write prompt 曾退化到只要求读取 `state.json` 并调用 `oz-exec`，没有明确 change 文档、acceptance 合同、required tests、task 完成标准和禁止改弱合同。artifact gate 可以补救漏产物，但首轮 prompt 必须先给足阶段合同，减少 agent 写错或漏写 artifact 的概率。

## 阶段提示词合同

提示词精简只允许发生在同一角色会话的第 2 轮及之后，用来省略重复 JSON 示例、方法论长文或历史背景；不得省略本轮目标 artifact、输入文件、禁止事项和验收边界。内置模板必须覆盖：

| 阶段模板 | 首轮必须包含 | 续轮允许省略 |
| --- | --- | --- |
| `wo-discuss.md` | 当前处于讨论规划阶段，并明确调用 `oz-plan` | 无主阶段 artifact，不强制续轮分支 |
| `wo-start.md` | `state.json`、change 目录、`proposal.md`、`design.md`、`spec.md`、`task.md`、`acceptance.json`、`tests/`、`required_tests`、不得删除/弱化/跳过契约、`oz status` task 完成标准、只实现当前 change | execution 没有可省略的首轮合同；如果后续增加 execution resume，也必须保留当前 change、acceptance 和 task 完成标准 |
| `wo-review.md` | baseline diff、change 文档、acceptance 合同、task 完成状态、测试/运行证据、`review-N.json` 输出路径、严格 JSON schema、clean/needs_fix 规则、parallel review 汇总规则 | JSON 示例和长 schema 示例；仍必须保留 `review-N.json`、上一轮 review/fix 引用和“只输出 JSON” |
| `wo-qa.md` | `review-N.json`、change 文档、`acceptance.json`、`required_tests`、`required_evidence`、真实用户路径复测、`acceptance_matrix`、`qa-N.json` 输出路径、不得修改源码或 acceptance | clean/needs_fix 示例；仍必须保留 `qa-N.json`、acceptance matrix 和 role session |
| `wo-fix.md` | 当前 review/QA findings、只修当前 findings、根因分析、禁止只按错误文本打补丁、不得删除/弱化 acceptance、验证命令、`fix-N-summary.md` 输出路径 | 根因分析方法论长文；仍必须保留当前 review/QA 路径、fix summary 路径和验证要求 |
| `wo-done.md` | 当前 run、`acceptance.json`、历史 review/QA/fix artifact、调用 `oz-archive`、最新 review/QA clean 条件、acceptance matrix 覆盖、`delivery-summary.md`、归档目录和 commit 范围 | 不适用；归档阶段必须始终保留可审计交付摘要合同 |

该合同需要有渲染级测试，不只检查模板源码。测试应覆盖首轮完整内容，以及 review/QA/fix 在已有 role session 时只省略示例/方法论、不省略目标 artifact。

## 统一阶段 artifact gate

实现应把“阶段运行后 artifact 未完成”和“artifact 格式/合同错误”统一建模为可重试的 stage artifact gate failure：

```text
runStage(stage)
  -> stageArtifactGate(stage)
       - missing artifact
       - invalid JSON/schema
       - acceptance_matrix 不完整
       - parallel gate 不满足
       - execution task 未完成
       - fix summary 缺失
       - archive delivery/目录缺失
  -> 失败:
       recordStageArtifactGateFailure(...)
       save state.validation[stage]
       resume 同一 role session
  -> 最多 3 次
```

建议把当前散落在 `artifactDone`、`nodeStageDone`、`advance` 和 `validateArchiveReadiness` 的错误转换收敛为一个清晰边界：

```text
internal/app/stage_artifact_gate.go
  +-- StageArtifactExpectation(stage)       // 给出阶段产物路径和业务说明
  +-- CheckStageArtifactGate(engine, state) // 返回 done 或 stageArtifactGateError
  +-- MissingStageArtifactError(...)        // 缺失产物也进入 retry
```

`recordStageArtifactGateFailure` 已经有 attempts、artifact 文件和 prompt 注入能力；本提案应复用它，不新增另一套重试计数。需要补强的是：

- `done=false` 必须转为 `stageArtifactGateError`，错误说明要包含阶段和期望产物。
- `validationFailurePrompt` 的 `Stage artifact gate failed` 分支必须列出目标产物路径和“只补写/改写当前阶段产物”的约束。
- `nodeRunStage` 和非 DAG `runLoop` 必须使用同一 artifact gate 结果，避免默认 go-dag 与旧 loop 行为分叉。

## 各阶段产物定义

```text
execution
  - oz status <change> --json 显示 tasks.total > 0 且 done == total
  - implementation_context required gate 通过

review_N
  - run/review-N.json 存在
  - JSON/schema 合法
  - parallel-review-N gate_input 未被 clean 忽略

qa_N
  - run/qa-N.json 存在
  - JSON/schema 合法
  - acceptance_matrix 覆盖 required_tests 和 required_evidence
  - parallel-qa-N gate_input 未被 clean 忽略

fix_N
  - run/fix-N-summary.md 存在且非空

archive
  - run/delivery-summary.md 存在且非空
  - docs/changes/archive/*-<change>/ 存在
  - review/QA/acceptance/validation 证据链仍满足 readiness
```

其中 `execution` 没有单独 JSON artifact，但 task 完成状态就是该阶段的业务产物。错误提示应直接说明“execution task 未完成”，并要求 executor 继续执行当前 oz change，而不是改 review/QA 或 archive。

## parallel member artifact 合同

parallel member artifact 需要一个统一边界：

```text
SUBAGENT_OUTPUT
  -> parseMemberArtifactStrict
  -> normalizeMemberArtifact
  -> validateMemberArtifact
  -> write normalized artifact
```

字段合同：

- `name`: 非空字符串，最终必须等于配置成员名。
- `purpose`: 非空字符串；缺失时可由配置补齐。
- `status`: 非空字符串；`success/passed/clean/completed/ok` 视为成功。
- `summary`: 非空字符串。
- `evidence`: 字符串数组。
- `findings`: 对象数组。
- `findings[].title/severity/evidence/recommendation`: 非空字符串。
- `required`: 由配置写回，不信任 agent 自填。

severity 策略：

```text
critical, blocker        -> blocker
high, medium, major      -> major
low, nit, minor          -> minor
info, informational,
note, warning            -> minor
其他值                  -> subagent artifact gate retry
```

`info` 不作为正式合同值，只作为输入别名归一为 `minor`。这样最终 group artifact 仍只有 `blocker/major/minor`，而真实 subagent 的低风险发现不会让批量任务中断。

## 风险和取舍

- 自动 retry 会让坏 agent 多跑几次；继续使用现有 `validation.max_attempts_per_stage: 3`，避免无限循环。
- `execution` task 未完成可能既是 agent 忘记更新 task，也可能是真实实现失败。前两次交给同一 executor 修正，第三次后阻断并暴露完整原因。
- `info -> minor` 会把提示性发现纳入低风险问题，不会触发 review/QA clean 硬阻断；这符合现有 gate 只阻断 `blocker/major` 的设计。
- fan-in 必须只汇总已经规范化的 member artifact，不能在 status 或 final gate 层临时容错，否则用户无法复核真实产物。
