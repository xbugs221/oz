---
name: oz-archive
description: 当用户提到 oz archive，或要求归档已完成的 oz 提案时使用；用于校验提案、确认测试、归档提案文档，并按逻辑把提案测试合并进 tests/specs/
---

# oz Archive

- 确认工作区干净或相关修改已提交，避免混入非提案文件
- 运行 `oz validate <change> --json`
- 确认 `task.md` 已完成
- 确认相关测试已经运行
- 先运行 `oz archive <change> --yes`，由 CLI 完成提案目录归档；CLI 不负责移动、改写或合并测试
- 读取 `docs/changes/archive/<date>-<change>/tests/`，理解每个测试表达的业务契约和断言
- 像合并 `docs/specs/*.md` 一样，把测试用例按业务能力合并到 `tests/specs/` 中稳定的规格测试文件；不要按 `<change>` 机械创建目录，也不要只搬运文件
- 合并后的规格测试文件开头可以批注相关来源提案，例如 `// Sources: 1-登录能力, 3-权限收敛`，但文件名和目录应表达能力而不是提案编号
- 重新运行受影响的 `tests/specs/` 规格测试入口，确认路径和测试执行都无误，再继续合并主规格或提交
- 读取 `docs/changes/archive/<date>-<change>/spec.md`，理解后合并到主规格 `docs/specs/*.md`
- 完成后，整理关联变动为一个 git commit，不要管无关内容，尤其不要干扰别的提案的文档
  - 如果之前的草稿提案 commit 已经推送，那么新建一个变更
  - 如果之前的草稿提案 commit 还在本地，那么合并成为一个整体
  - commit message 格式： "<number>: <change-name>"
- 交付时说明归档路径、逻辑合并后的规格测试文件、主规格合并文件、运行过的命令和剩余风险
