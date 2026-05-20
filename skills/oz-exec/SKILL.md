---
name: oz-exec
description: 当用户提到 oz exec，或要求执行一个活动 oz 提案时使用；用于读取提案产物、编写真实测试、按新意图更新过期历史测试并实现变更
---

# oz Exec

执行当前 `oz` 变更中的实现任务

确认提案目录已提交到 git，防止后续操作误删：

```
git log --oneline -- docs/changes/<change>/
```

若未提交，先 `git add docs/changes/<change>/ && git commit -m "提案草稿: <change>"`。

先读取：

- `proposal.md`
- `design.md`
- `spec.md`
- `task.md`
- `tests/` 中创建阶段已经写好的验收测试

实现时：

- 以当前提案和用户最新意图为准
- 审查根目录 `tests/` 中与本次变更相关的历史测试
- 如果历史测试与新意图冲突，更新测试代码，并在 `design.md` 或 `task.md` 记录原因
- 先运行创建阶段写入 `docs/changes/<change>/tests/` 的验收测试；如果功能尚未实现，失败原因应指向目标行为缺失
- 不得删除、弱化、跳过或改写创建阶段的验收测试来让实现过关
- 如用户最新意图明确改变验收标准，必须先同步更新 `spec.md`、`design.md`、`task.md` 和对应测试，并写明变更原因，再继续实现
- 可以新增补充测试，但新增测试必须是真实项目测试代码，先写入 `docs/changes/<change>/tests/`
- 不得 mock API、mock 数据库、伪造认证、硬编码成功结果或只断言 HTTP 200，除非用户明确要求且已在提案文档记录风险
- 不在 `tests/` 写占位文档
- 完成任务后更新 `task.md` 复选框
- 结束前运行相关测试

交付时说明实现内容、测试变更、历史测试更新原因、运行过的命令和剩余风险
