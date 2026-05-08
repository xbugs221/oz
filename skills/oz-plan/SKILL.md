---
name: oz-plan
description: 当用户提到 oz plan，或希望在创建提案文件前规划一个 oz 变更时使用；用于澄清范围、可行性、风险、测试策略和中文 change-name。
---

# oz Plan

围绕用户意图讨论：

- 解决什么问题，为什么现在需要做
- 哪些行为、文档、命令或测试会变化
- 哪些内容明确不在本次范围内
- 需要新增或更新哪些真实测试代码
- 是否会影响历史测试，是否需要按新意图更新旧测试
- 多用ascii图和树状图形式阐述说明

规划结束时给出：

- `change-name`：需求描述，可混用英文、数字和连字符，但不能全英文
- `problem`：问题和动机
- `scope`：本次范围
- `non-goals`：非目标
- `tests`：执行阶段应写入 `docs/changes/<change>/tests/` 的真实测试代码
- `open questions`：无法合理假设的阻塞问题
