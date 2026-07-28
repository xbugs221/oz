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
- 禁止创建或修改 `task.md`；归档完成条件以校验、验收结果和最终审核为准
- 重新运行相关测试并确认最终通过轮次；`test-results/` 只作临时目录
- Oz 工作流上下文中，引擎会在进入本阶段前从最终通过的不可变副本生成 `tests/evidence/proposals/<change>/`；只核对 `README.md`、`manifest.json`、demo、日志及 `submission_evidence` 映射，不得重新复制或覆盖
- 独立归档上下文中，在调用 `oz archive` 前将最终通过轮次的证据筛选、脱敏并归集到同一目录，同时生成 `README.md` 与 `manifest.json`
- 先运行 `oz archive <change> --yes`，由 CLI 完成提案目录（含 `tests/`）迁移，并自动改写测试脚本、`acceptance.json`、`brief.md`、`spec.md` 等文本中的提案测试路径；CLI 不负责把测试按业务能力合并进长期 `tests/specs/`
- 读取 `docs/changes/archive/<date>-<change>/tests/`，理解每个测试表达的业务契约和断言
- 像合并 `docs/specs/*.md` 一样，把测试用例按业务能力合并到 `tests/specs/` 中稳定的规格测试文件；不要按 `<change>` 机械创建目录，也不要只搬运文件
- 合并后的规格测试文件开头可以批注相关来源提案，例如 `// Sources: 1-登录能力, 3-权限收敛`，但文件名和目录应表达能力而不是提案编号
- 重新运行受影响的 `tests/specs/` 规格测试入口，确认路径和测试执行都无误，再继续合并主规格或提交
- 读取 `docs/changes/archive/<date>-<change>/spec.md`，理解后合并到主规格 `docs/specs/*.md`
- 只用明确路径逐文件暂存，不得使用 `git add .`、`git add -A` 或 `git add -f`。`test-results/` 必须保持忽略；`tests/evidence/proposals/<change>/` 必须可跟踪，若被宽泛规则误伤，只添加最窄的 `.gitignore` 例外
- 提交前运行 `git diff --cached --name-only` 获取完整暂存清单，并对每个候选路径运行 `git check-ignore --no-index -- <path>`；移除意外命中的路径，同时确认最终证据目录已正常暂存
- 单个证据文件超过 20 MiB 时必须配置 Git LFS，并用 `git check-attr filter -- <path>` 确认 `filter=lfs`
- 完成后，以工作流启动时封存的 `delivery_base_head` 为基点新建且只新建一个完整交付 commit；不要 amend、rebase、squash 或改写基点前历史，也不要管无关内容
  - 归档目录、代码、测试、长期规格和 `tests/evidence/proposals/<change>/` 必须位于同一提交
  - `delivery_base_head..HEAD` 必须恰好一个提交，且 HEAD 就是上述完整交付提交
  - commit message 格式： "<number>: <change-name>"
- 交付时说明归档路径、逻辑合并后的规格测试文件、主规格合并文件、运行过的命令和剩余风险

## 退出条件

归档阶段只有在以下条件同时满足时才算完成：

- `oz validate <change> --json` 通过，验收测试与最终审核已经完成
- 归档后的提案目录存在于 `docs/changes/archive/`
- 提案 `tests/` 的业务意图已按能力合并到稳定的 `tests/specs/` 文件，或明确说明无需合并的原因
- 归档 `spec.md` 的长期行为已合并到 `docs/specs/*.md`，或明确说明无需合并的原因
- 受影响规格测试已经重新运行
- `tests/evidence/proposals/<change>/` 已包含最终通过轮次的 demo 视频、`README.md` 摘要和必要日志，并与代码处于同一提交
- `delivery_base_head..HEAD` 恰好一个完整交付提交，提交后相关工作区干净
- 暂存清单已经逐项复核，不含 `test-results/`、失败轮次、缓存、敏感或无关产物

## 反偷懒检查

| 常见偷懒理由 | 处理方式 |
| --- | --- |
| “CLI 已经 archive，事情结束” | CLI 只移动提案目录；长期规格和测试仍要人工逻辑合并 |
| “按 change 编号复制测试最省事” | 长期测试按业务能力组织，不按提案编号机械分组 |
| “历史测试看起来重复，可以不读” | 先理解断言和真实入口，再决定合并、改写或说明无需合并 |
| “工作区有别的改动也一起提交” | 只整理当前归档相关变动，不碰无关文件 |
| “直接提交 test-results 最省事” | 只把最终通过轮次筛选到 `tests/evidence/proposals/<change>/`；禁止提交整个临时目录 |
