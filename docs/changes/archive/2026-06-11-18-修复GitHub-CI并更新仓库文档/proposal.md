# 提案：修复 GitHub CI 并更新仓库文档

## 背景

GitHub Actions 上 `CI` workflow 最近 4 次运行失败。最新可观察失败是：

- run：`27288329734`
- URL：`https://github.com/xbugs221/oz/actions/runs/27288329734`
- 分支：`main`
- 提交：`7fd5b2780da48c384e89d5987aa67620f01939fc`
- 标题：`subagent defaults to pi`
- 失败 step：`Run Go tests`

在 `7fd5b278` 的临时 worktree 中运行 `go test ./...`，失败点为：

- `TestParallelEnabledPromptsCarryFanoutArtifacts/planning`
- `TestBundledOzSkillPromptsDelegateToSkills`

两处都说明规划 prompt 缺少 `讨论规划阶段`。当前 bundled prompt 内容是 `调用 \`oz-plan\` 技能，开始讨论规划`，只保留了技能名，没有保留历史合同和长期规格依赖的阶段语义。

## 要做什么

1. 修复规划阶段 prompt 合同，让 GitHub CI 的 Go 测试恢复通过。
2. 保留 `oz-plan` 技能入口和“讨论规划阶段”这类用户可读阶段语义，避免执行器只看到一个技术 skill 名称。
3. 更新仓库文档，明确本仓库 GitHub CI 和 Release 的测试门禁、真实复现命令、失败排查入口，以及 prompt 合同与长期规格的关系。
4. 验证 CI 和 Release 共享的本地门禁仍然是 `go test ./...` 加根目录 `tests/*.sh` 业务测试。

## 为什么现在做

CI 已经在 GitHub `main` 上阻断。继续堆叠功能提交会让失败归因变难，也会让 README 对当前维护流程的描述落后于实际 GitHub Actions。此变更优先恢复可信门禁，再补齐文档，让后续提案执行和归档有稳定基础。
