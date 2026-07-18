# 强化验收硬合同并精简执行上下文

本变更把 oz 提案的质量权威从长篇描述文档转移到 `acceptance.json`、契约测试和证据矩阵。

用户主要阅读 `brief.md` 了解本次变更解决什么问题、改变什么行为和剩余风险；执行智能体默认只读取硬合同和契约测试，长篇设计背景仅在冲突或歧义时按需读取。

验收重点：

- `oz validate` 拒绝缺少业务断言、使用弱断言或测试路径不受合同约束的提案。
- execution prompt 默认聚焦 `brief.md`、`acceptance.json` 和 `docs/changes/archive/2026-06-10-8-强化验收硬合同并精简执行上下文/tests/`。
- `oz status --json` 暴露 `brief.md` 和验收合同摘要，避免只靠 task checkbox 判断就绪。

非目标：本变更不删除现有 `proposal.md`、`design.md`、`spec.md`、`task.md`，只降低它们在执行阶段的默认注意力权重。
