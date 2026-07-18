# 任务

## 1. 契约测试

- [x] 1.1 先运行 `bash docs/changes/archive/2026-06-11-18-修复GitHub-CI并更新仓库文档/tests/test_github_ci_prompt_contract.sh`，确认当前实现会因为 planning prompt 合同缺失而失败。
- [x] 1.2 先运行 `bash docs/changes/archive/2026-06-11-18-修复GitHub-CI并更新仓库文档/tests/test_ci_documentation_contract.sh`，确认 README 尚未完整说明 CI/Release 门禁时会失败。

## 2. 修复 CI 失败

- [x] 2.1 用 `gh run view 27288329734 --repo xbugs221/oz --json ...` 或等价方式记录 GitHub 失败 run 的 job/step 摘要。
- [x] 2.2 修复 `wo-discuss` bundled prompt、相关长期规格或测试之间的不一致，保留 `oz-plan` 和 `讨论规划阶段` 两个用户可感知语义。
- [x] 2.3 运行定向 app Go 合同测试，确认 prompt 合同通过且不是 no-op。
- [x] 2.4 运行完整 `go test ./...`，确认 GitHub `Run Go tests` step 的本地等价门禁通过。

## 3. 更新仓库文档

- [x] 3.1 更新 README，增加 GitHub Actions CI/Release 门禁、本地复现命令和失败排查入口。
- [x] 3.2 必要时更新 `docs/specs/release-automation/spec.md` 或 `docs/specs/codex-workflow-cli/spec.md`，保持长期规格与 workflow 一致。
- [x] 3.3 确认 `.github/workflows/ci.yml` 和 `.github/workflows/release.yml` 仍使用本地构建出的 `oz`、`wo`，并运行 `go test ./...` 与根目录 `docs/changes/archive/2026-06-11-18-修复GitHub-CI并更新仓库文档/tests/*.sh`。

## 4. 验证和证据

- [x] 4.1 重新运行本提案 `docs/changes/archive/2026-06-11-18-修复GitHub-CI并更新仓库文档/tests/` 下两个契约测试。
- [x] 4.2 运行根目录 shell 业务测试门禁：遍历执行 `docs/changes/archive/2026-06-11-18-修复GitHub-CI并更新仓库文档/tests/*.sh`，或使用仓库已有等价门禁脚本。
- [x] 4.3 如果代码已推送，记录新的 GitHub `CI` 成功 run URL；如果未推送，在 QA artifact 中明确说明只完成本地等价门禁。
- [x] 4.4 保存 `test-results/18-github-ci-docs/` 下的 runtime log 和 GitHub run 摘要，供 review/QA 复核。

## 执行记录

- 历史测试更新原因：当前长期规格已支持 `agy` agent backend，多个根目录业务测试和迁移 app 测试使用 fake backend 集合运行真实 `wo` 路径，因此补齐 fake `agy`，避免本地/CI 门禁依赖开发机安装真实 `agy`。
- 迁移测试更新原因：`docs/changes/archive/2026-06-11-18-修复GitHub-CI并更新仓库文档/tests/app` harness 的职责是用生产代码执行迁移后的 `.gotest`，不应重复复制并运行 `internal/app/*_test.go`；原包测试继续由 `go test ./internal/app` 覆盖。
- 本次未推送代码，未产生新的 GitHub CI 成功 run URL；已完成本地等价门禁。
