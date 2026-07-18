# 修复 GitHub CI 并更新仓库文档

GitHub `xbugs221/oz` 的最新 CI 在 `2026-06-10` 连续失败，最新失败 run 是 `27288329734`，分支 `main`，提交 `7fd5b2780da48c384e89d5987aa67620f01939fc`，失败 step 是 `Run Go tests`。创建阶段在该提交的临时 worktree 复现到 `internal/app` 的规划提示词合同失败：默认 `wo-discuss` prompt 只包含 `oz-plan`，但缺少既有测试和长期规格要求的“讨论规划阶段”语义。

本次变更要修复 GitHub CI 中的 Go 测试失败，并同步更新 README、长期规格或门禁说明，让维护者能从仓库文档直接知道 CI/Release 会运行哪些命令、如何本地复现 GitHub 失败、以及 prompt 合同为什么不能只保留技能名。

本次不重构 GitHub Actions 发布流程，不更换 Actions 版本，不新增外部 CI 服务，也不把失败测试弱化为只检查文件存在。若执行阶段判断长期合同本身过时，必须同时更新主规格、测试和文档，并保留规划阶段可读语义。

验收入口：

- `docs/changes/archive/2026-06-11-18-修复GitHub-CI并更新仓库文档/tests/test_github_ci_prompt_contract.sh`
- `docs/changes/archive/2026-06-11-18-修复GitHub-CI并更新仓库文档/tests/test_ci_documentation_contract.sh`
