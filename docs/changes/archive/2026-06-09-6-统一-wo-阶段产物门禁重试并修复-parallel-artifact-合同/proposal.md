# 统一 wo 阶段产物门禁重试并修复 parallel artifact 合同

## 问题

`wo` 的默认 `go-dag` 工作流已经会在部分 review/QA schema 错误后记录 `Stage artifact gate failed` 并重跑同一阶段，但覆盖不完整。实际批量执行中仍会出现这类失败：

- `execution 阶段 artifact 未完成`
- `parallel artifact member 0 finding 0 的 severity 无效："info"`
- execution/write 等主阶段首轮提示词被过度精简，只剩读取 `state.json` 和调用技能，导致 agent 不知道必须读取 proposal/design/spec/task/acceptance、运行契约测试和写出阶段产物。

这些问题都不是业务实现已经确认失败，而是当前阶段或并行成员没有按 `wo` 要求补齐产物，或者写出了可修正的低级字段错误。现在的行为会直接让 run/batch failed，用户只能手动 `wo restart -b1` 清掉失败记录再继续，批量任务因此被低级产物错误中断。

## 目标

- 任意主阶段运行完成后都必须立即检查该阶段应有产物。
- 产物缺失、格式错误或合同不满足时，不直接 failed；系统必须记录 artifact gate failure，并 resume 同一角色会话要求只补写或改写当前阶段产物。
- 同一阶段 artifact gate 修正最多 3 次，超过后才进入阻断状态，错误必须包含阶段、产物路径和失败原因。
- `execution` task 未完成、`fix` summary 缺失、`archive` delivery summary 或归档目录缺失，都纳入同一 artifact gate retry，而不是直接 failed。
- parallel subagent member artifact 使用统一 normalize+validate 边界，`nodeRunSubagent` 和 fan-in 读取都必须复用。
- parallel member `findings[].severity` 最终只存 `blocker`、`major`、`minor`；`info`、`informational`、`note`、`warning` 等低风险口径归一为 `minor`。
- subagent artifact 无法归一的语义错误必须 resume 同一 subagent session，最多 3 次，提示只重写 `SUBAGENT_OUTPUT`。
- 所有主阶段首轮提示词必须保留完整阶段合同：输入文件、阶段职责、禁止事项、输出产物、验收/证据边界和失败修正路径；只有同一角色续轮可以省略重复示例和方法论，不能省略当前目标 artifact。
- 更新 prompt 和公开规格，让主 agent 和 subagent 明确产物字段、路径、severity 枚举、首轮/续轮边界和 retry 约束。

## 非目标

- 不放宽最终 `review-N.json`、`qa-N.json`、acceptance matrix 或 archive readiness 的严格合同。
- 不改变 review/QA/fix/archive 的主状态机语义。
- 不让 status 层吞掉非法 artifact 并显示 success。
- 不新增 workflow engine、外部 scheduler 或新的 agent backend。
- 不允许 required parallel member 失败后伪装成功继续推进。
- 不把 `oz-plan`、`oz-exec`、`oz-archive` 技能全文复制进模板；模板应保留阶段合同并委托技能处理具体流程。

## 验收

本提案通过以下真实测试和回归测试验收：

- `docs/changes/archive/2026-06-09-6-统一-wo-阶段产物门禁重试并修复-parallel-artifact-合同/tests/test_stage_artifact_gate_retry_all_roles.sh`
- `docs/changes/archive/2026-06-09-6-统一-wo-阶段产物门禁重试并修复-parallel-artifact-合同/tests/test_batch_continues_after_stage_artifact_repair.sh`
- `docs/changes/archive/2026-06-09-6-统一-wo-阶段产物门禁重试并修复-parallel-artifact-合同/tests/test_parallel_subagent_info_severity_contract.sh`
- `docs/changes/archive/2026-06-09-6-统一-wo-阶段产物门禁重试并修复-parallel-artifact-合同/tests/test_stage_prompt_contract_completeness.sh`
- `go test ./...`

这些测试使用真实构建出的 `wo` 二进制、真实 `go-dag` 入口、临时 git 仓库、fake `codex/pi/oz` CLI 和真实状态目录。执行阶段不得通过删除测试、放宽断言、禁用 artifact gate 或绕过 batch 状态来通过验收。
