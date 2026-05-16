---
name: oz-create
description: 当用户提到 oz create，或要求创建 oz 变更提案时使用；用于创建 docs/changes/<编号>-<中文提案名>/ 及 proposal.md、design.md、spec.md、task.md、tests/
---

# oz Create

创建 `oz` 变更提案并生成所有产物。不要只写四个 Markdown 文件；`tests/` 也是提案结构的一部分，必须在创建阶段落盘。

- `proposal.md`：说明做什么和为什么
- `design.md`：说明关键技术决策、取舍和风险
- `spec.md`：使用中文 `### 需求：` 和 `#### 场景：` 描述验收行为
- `task.md`：拆分实现步骤和验证条件
- `tests/`：必须创建为空目录，保留给执行阶段放真实测试代码，不写测试说明文档或占位文件

创建提案时必须把测试意图写进文档，而不是写进 `tests/` 目录：

- `spec.md` 的 `#### 场景：` 覆盖用户可感知的验收行为
- `design.md` 说明需要新增或更新哪些真实测试代码，以及为什么这些测试能证明行为正确
- `task.md` 包含实现后运行或补充测试的任务项
- 如果暂时无法确定测试策略，先和用户澄清；不要创建缺少测试场景的提案

目录名为 `docs/changes/<number>-<change-name>/`，不要写日期前缀:

- `<number>` 取活动和归档提案中的最大数字前缀加一
- `<change-name>` 必须是中文需求描述，可以混用英文单词、数字和连字符，但必须包含中文汉字，不能全英文

然后运行 `oz validate <change> --json` 检验，确认 `tests/` 目录存在且没有占位内容。
