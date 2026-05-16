---
name: oz-archive
description: 当用户提到 oz archive，或要求归档已完成的 oz 提案时使用；用于校验提案、确认测试、将测试代码按来源命名移动到根 tests/，并归档提案文档
---

# oz Archive

- 确认工作区干净或相关修改已提交，避免混入非提案文件
- 运行 `oz validate <change> --json`
- 确认 `task.md` 已完成
- 确认相关测试已经运行
- 先运行 `oz archive <change> --yes`，由 CLI 完成测试文件移动和提案目录归档
- 归档后测试文件位于根目录 `tests/`，文件名带 `<date>-<change>-` 前缀，例如 `tests/2026-05-08-2-重写-oz-cli-test-ui-1.ts`。审查涉及路径的测试入口，测试必须按归档后的根 `tests/` 位置改写
- 重新运行被移动到根 `tests/` 的测试入口，确认路径仍然正确
- 读取 `docs/changes/archive/<date>-<change>/spec.md`，理解后合并到主规格 `docs/specs/*.md`
- 完成后，整理关联变动为一个 git commit，不要管无关内容，尤其不要干扰别的提案的文档
  - 如果之前的草稿提案 commit 已经推送，那么新建一个变更
  - 如果之前的草稿提案 commit 还在本地，那么合并成为一个整体
  - 上述两种情况下，提交信息统一写提案编号和名称，例如 19: 精简身份体系
- 交付时说明归档路径、移动后的测试文件、主规格合并文件、运行过的命令和剩余风险
