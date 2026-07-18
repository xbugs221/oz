# 规格

## 验收矩阵

| 需求 | 场景 | 测试 | 证据 | 风险 |
| --- | --- | --- | --- | --- |
| GitHub CI Go 测试恢复通过 | planning prompt 合同与长期规格一致 | `github-ci-prompt-contract` | `github-ci-prompt-log`, `github-ci-run-summary` | GitHub 日志正文可能不可下载，使用 run/job JSON 和本地复现日志补足 |
| 仓库文档说明 CI/CD 门禁 | README 与长期规格列出真实 CI/Release 命令 | `ci-documentation-contract` | `ci-documentation-log` | 测试只验证文档和 workflow 的关键命令，不评价文档措辞质量 |

### 需求：GitHub CI Go 测试恢复通过

系统必须修复 GitHub `CI` workflow 在 `Run Go tests` step 的失败，使规划阶段 bundled prompt 同时保留 `oz-plan` 技能入口和“讨论规划阶段”这类用户可读阶段语义。

#### 场景：planning prompt 合同与长期规格一致

- **给定** GitHub 最新失败 run `27288329734` 在提交 `7fd5b2780da48c384e89d5987aa67620f01939fc` 上失败于 `Run Go tests`
- **并且** 本地复现显示 `TestParallelEnabledPromptsCarryFanoutArtifacts` 和 `TestBundledOzSkillPromptsDelegateToSkills` 都缺少 `讨论规划阶段`
- **当** 执行阶段修复 prompt、测试或规格的真实不一致
- **则** `wo-discuss` bundled prompt 必须同时包含 `oz-plan` 和 `讨论规划阶段`
- **并且** 定向 app Go 合同测试必须通过
- **并且** 收尾必须运行完整 `go test ./...`
- **并且** GitHub 上新的 `CI` run 必须不再失败于同一 prompt 合同
- **测试**：`docs/changes/archive/2026-06-11-18-修复GitHub-CI并更新仓库文档/tests/test_github_ci_prompt_contract.sh`
- **真实数据来源**：GitHub run `27288329734` 的 job/step 摘要、当前仓库真实 `prompts-template/wo-discuss.md`、真实迁移 app Go 测试或旧 `internal/app` Go 测试
- **入口路径**：`go test ./tests/app` 或 `go test ./internal/app`
- **关键断言**：prompt 包含 `oz-plan`；prompt 包含 `讨论规划阶段`；相关 Go 合同测试通过且不是 no-op
- **剩余风险**：该测试不主动推送代码触发 GitHub Actions；最终 GitHub 成功 run 由 QA 阶段提供 URL 或 `gh run view --json` 证据

### 需求：仓库文档说明 CI/CD 门禁

系统必须让维护者从 README 和长期规格中看到 GitHub CI/Release 的真实测试门禁和本地复现入口，避免 workflow、规格和用户文档互相漂移。

#### 场景：README 与长期规格列出真实 CI/Release 命令

- **给定** 仓库存在 `.github/workflows/ci.yml` 和 `.github/workflows/release.yml`
- **当** 执行阶段更新仓库文档
- **则** README 必须说明 GitHub Actions、CI、Release、`go test ./...` 和根目录 `docs/changes/archive/2026-06-11-18-修复GitHub-CI并更新仓库文档/tests/*.sh`
- **并且** README 必须给出本地复现 GitHub CI 失败的入口
- **并且** 长期规格必须继续说明 CI/Release 使用本地 `oz`、`wo` 构建和同一套测试门禁
- **并且** workflow 文件仍必须运行 `go test ./...` 和遍历根目录 `docs/changes/archive/2026-06-11-18-修复GitHub-CI并更新仓库文档/tests/*.sh`
- **测试**：`docs/changes/archive/2026-06-11-18-修复GitHub-CI并更新仓库文档/tests/test_ci_documentation_contract.sh`
- **真实数据来源**：真实 README、`docs/specs/release-automation/spec.md`、`.github/workflows/ci.yml`、`.github/workflows/release.yml`
- **入口路径**：`bash docs/changes/archive/2026-06-11-18-修复GitHub-CI并更新仓库文档/tests/test_ci_documentation_contract.sh`
- **关键断言**：README 写明 CI/Release 命令；release automation 规格写明同一门禁；两个 workflow 文件保留真实测试命令
- **剩余风险**：测试不验证 GitHub UI 文案，只验证仓库内可版本化的文档和 workflow
