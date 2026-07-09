---
name: oz-archive
description: 当用户提到 oz archive，或要求归档已完成的 oz 提案时使用；用于校验提案、确认测试、归档提案文档，并按逻辑把提案测试合并进 tests/specs/
---

# oz Archive

归档阶段的目标不是把目录搬走，而是把已经验证过的 change 意图沉淀到长期规格和长期规格测试里。

## 入口差异

| 类型 | 归档读取重点 | 长期沉淀责任 |
| --- | --- | --- |
| small | `brief.md`、`acceptance.json`、`tests/`，即 brief-only | 必须从 brief 提取长期行为，合并到 `docs/specs/`，并把测试意图合并到 `tests/specs/` |
| standard | 完整提案文档、`acceptance.json`、`tests/` | 从 `spec.md` 和测试中合并长期规格与长期规格测试 |

## 流程

- 确认工作区干净或相关修改已提交，避免混入非提案文件
- 运行 `oz validate <change> --json`
- standard 确认 `task.md` 已完成；small brief-only 没有 `task.md` 时不补写任务文件
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

## 退出条件

归档阶段只有在以下条件同时满足时才算完成：

- `oz validate <change> --json` 通过，且 standard 的 `task.md` 已完成；small brief-only 没有 `task.md` 时不适用
- 归档后的提案目录存在于 `docs/changes/archive/`
- 提案 `tests/` 的业务意图已按能力合并到稳定的 `tests/specs/` 文件，或明确说明无需合并的原因
- 归档 `spec.md` 的长期行为已合并到 `docs/specs/*.md`，或明确说明无需合并的原因
- 受影响规格测试已经重新运行

## 反偷懒检查

| 常见偷懒理由 | 处理方式 |
| --- | --- |
| “CLI 已经 archive，事情结束” | CLI 只移动提案目录；长期规格和测试仍要人工逻辑合并 |
| “按 change 编号复制测试最省事” | 长期测试按业务能力组织，不按提案编号机械分组 |
| “历史测试看起来重复，可以不读” | 先理解断言和真实入口，再决定合并、改写或说明无需合并 |
| “工作区有别的改动也一起提交” | 只整理当前归档相关变动，不碰无关文件 |
