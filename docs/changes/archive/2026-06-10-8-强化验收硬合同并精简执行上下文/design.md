# 设计

## 背景

这个变更的目标不是让提案文档更完整，而是让执行阶段更少依赖长文本、更依赖可运行合同。`oz-create` skill 仍负责生成提案，但质量门槛必须落到 CLI 校验、调度提示词和状态 artifact 上。

## 决策

### 1. 保留旧文件，新增 `brief.md`

`proposal.md`、`design.md`、`spec.md` 和 `task.md` 暂时保留，避免一次性破坏已有流程。`brief.md` 成为用户优先阅读的短文档；执行阶段默认只把它当上下文摘要，不再默认展开所有长文档。

### 2. `acceptance.json` 是硬合同入口

`acceptance.json` 继续使用当前字段，但提高校验强度：

- `required_tests[].assertions` 不能为空。
- 断言不能只写 `HTTP 200`、`元素存在`、`组件渲染成功` 这类弱表面信号。
- `required_tests[].path` 必须指向真实测试文件。
- `required_tests[].command` 必须引用对应测试路径，避免合同路径和执行命令脱节。
- `coverage[]` 必须引用真实存在的测试和证据 ID。

### 3. `oz validate` 负责拒绝弱合同

skill 是生成建议，不能作为硬门槛。`oz validate` 必须在实现前拒绝弱测试合同，确保执行器拿到的硬合同已经具备最低质量。

### 4. execution prompt 默认聚焦硬合同

默认 execution prompt 应要求读取：

- `state.json`
- 当前 change 的 `brief.md`
- 当前 change 的 `acceptance.json`
- 当前 change 的 `tests/`

`proposal.md`、`design.md`、`spec.md`、`task.md` 只在以下情况下按需读取：验收合同和用户最新意图冲突、历史测试需要更新、实现路径存在会影响架构的分歧、或 review/QA 证据指向长文档中的风险。

### 5. `oz status --json` 暴露合同摘要

`oz status --json` 不能只依赖 task checkbox。它应继续报告 artifact 和 task 状态，同时增加验收合同摘要，让调度器和人类能看到硬合同规模：

- `acceptance.coverage.total`
- `acceptance.required_tests.total`
- `acceptance.required_evidence.total`

## 风险

- 弱断言识别如果做成复杂语义判断，容易误伤。第一版只拒绝明确弱模式和缺断言。
- 仅靠测试仍无法证明所有质量属性，review 仍要检查硬编码、绕过合同和实现风险。
- 旧提案没有 `brief.md`，实现时需要明确只对 active change 校验，或提供兼容策略。

## 执行记录

- 历史测试 helper `acceptanceJSON()` 原本生成无 `assertions` 的旧合同；本变更将缺断言定义为无效合同，因此已同步补充业务级断言，避免历史批处理测试继续依赖过期弱合同。
