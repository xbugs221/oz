---
name: oz-create
description: 当用户提到 oz create，或要求创建 oz 变更提案时使用；用于创建 docs/changes/<编号>-<中文提案名>/ 及 proposal.md、design.md、spec.md、task.md、tests/
---

# oz Create

创建 `oz` 变更提案并生成所有产物

- `proposal.md`：说明做什么和为什么
- `design.md`：说明关键技术决策、取舍和风险
- `spec.md`：使用中文 `### 需求：` 和 `#### 场景：` 描述验收行为
- `task.md`：拆分实现步骤和验证条件
- `tests/`：保留给执行阶段放真实测试代码，不写测试说明文档

目录名为 `docs/changes/<number>-<change-name>/`：

- `<number>` 取活动和归档提案中的最大数字前缀加一
- `<change-name>` 必须是中文需求描述，可以混用英文单词、数字和连字符，但必须包含中文汉字，不能全英文

创建后运行 `oz validate <change> --json` 检验
