# 提案：收窄验收 gate 到提案范围

## 问题

实测中，oz 提案进入 review、QA 和 parallel gate 后，agent 经常把当前 diff 附近看到的历史债务也按 blocker/major finding 纳入硬阻断。结果是当前提案虽然已经满足 `acceptance.json` 和用户目标，却因为无关历史问题无法结束。

当前机制里 QA 的 `acceptance_matrix` 已经绑定 `acceptance.json`，但 review/parallel finding 只有 `title`、`severity`、`evidence` 和 `recommendation`，缺少机器可判断的 scope。只要 gate_input 成员报告 blocker/major finding 或成员失败，clean 就会被阻止。这让“发现历史债务”和“当前提案未完成”在 gate 上没有清晰边界。

## 变更

- 为 review、QA 和 parallel finding 增加 scope 分类。
- 新增非阻断 finding 承载区，让 clean artifact 能记录既有历史债务。
- gate 只把当前提案范围内的 blocker/major finding 作为硬阻断。
- prompt 明确要求 reviewer/QA 先判断 finding 是否属于当前提案。
- 保持旧格式兼容：旧提案、旧 acceptance 合同和缺少 scope 的旧 finding 不需要迁移。

## 非目标

- 不把历史债务自动转成代码修复。
- 不新增 backlog 管理系统。
- 不改变 `acceptance.json` 的结构。
- 不改变 `oz create` 的编号和归档流程。

## 验收

- clean review 可以包含 scope 为 `out_of_scope_existing` 的非阻断历史债务。
- parallel review/QA artifact 中 scope 为 `out_of_scope_existing` 的 severe finding 不阻断 clean。
- scope 为 `current_change`、`acceptance_contract` 或 `introduced_regression` 的 severe finding 仍阻断 clean。
- 缺少 scope 的旧 finding 默认保持旧行为，仍按当前提案范围内问题处理。
- 未运行旧提案不增加新必填字段，继续通过 `oz validate`。
