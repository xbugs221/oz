---
name: oz-exec
description: 当用户提到 oz exec，或要求执行一个活动 oz 提案时使用；用于读取提案产物、编写真实测试、按新意图更新过期历史测试并实现变更
---

# oz Exec

执行当前 `oz` 变更中的实现任务

先读取：

- `proposal.md`
- `design.md`
- `spec.md`
- `task.md`

实现时：

- 以当前提案和用户最新意图为准
- 审查根目录 `tests/` 中与本次变更相关的历史测试
- 如果历史测试与新意图冲突，更新测试代码，并在 `design.md` 或 `task.md` 记录原因
- 新增测试必须是真实项目测试代码，先写入 `docs/changes/<change>/tests/`
- 不在 `tests/` 写占位文档
- 完成任务后更新 `task.md` 复选框
- 结束前运行相关测试

交付时说明实现内容、测试变更、历史测试更新原因、运行过的命令和剩余风险
