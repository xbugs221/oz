# 强化验收硬合同并精简执行上下文

## 背景

当前 oz 提案把需求背景、设计取舍、行为规格、任务清单、验收合同和测试同时交给执行智能体阅读。实践上，真正能锁住质量的是可运行测试、业务级断言和可复核证据；长篇文字即使写得漂亮，也容易承诺超过测试覆盖的范围，并稀释执行阶段注意力。

本变更要把提案拆成两层：

- 面向用户的简短说明：`brief.md`，只解释关键信息、行为变化、非目标和风险。
- 面向智能体和调度器的硬合同：`acceptance.json`、`docs/changes/archive/2026-06-10-8-强化验收硬合同并精简执行上下文/tests/` 和运行证据，决定执行、review、QA 是否能通过。

## 变更内容

- 新增 `brief.md` 作为 active change 的用户简报 artifact，并让 `oz status --json` 暴露它。
- 强化 `acceptance.json` 校验：`required_tests[].assertions` 必须包含业务级断言，弱断言和缺断言不能通过。
- 强化 `oz validate` 对测试路径和验收矩阵的交叉校验，避免合同声明和真实测试脱节。
- 调整 execution 默认提示词：默认读取 `brief.md`、`acceptance.json` 和 `docs/changes/archive/2026-06-10-8-强化验收硬合同并精简执行上下文/tests/`，只在冲突、歧义、历史测试不一致时按需读取 `proposal.md`、`design.md`、`spec.md`、`task.md`。
- 让 `oz status --json` 暴露验收合同摘要，包括 required tests、required evidence 和 coverage 数量。

## 非目标

- 不删除现有 Markdown 提案文件，避免破坏已有归档和 skill 兼容性。
- 不把所有质量问题都交给静态文本校验解决；核心质量仍来自真实测试运行和 QA 证据。
- 不引入新的测试框架，不改变仓库现有 Go 与 shell 测试入口。
