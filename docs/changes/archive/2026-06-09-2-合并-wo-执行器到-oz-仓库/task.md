# 任务

## 1. 契约测试基线

- [x] 1.1 运行 `bash docs/changes/archive/2026-06-09-2-合并-wo-执行器到-oz-仓库/tests/test_monorepo_cli_release_contract.sh`，确认当前实现因缺少 `cmd/wo`、`internal/app` 或 CI 本地构建约束而失败。
- [x] 1.2 运行 `bash docs/changes/archive/2026-06-09-2-合并-wo-执行器到-oz-仓库/tests/test_oz_acceptance_validation_contract.sh`，确认当前实现因 `oz validate` 未要求 `acceptance.json` 而失败。

## 2. 合入 wo 源码和规格

- [x] 2.1 将 `../wo/cmd/wo` 合入当前仓库的 `cmd/wo`。
- [x] 2.2 将 `../wo/internal/app` 合入当前仓库的 `internal/app`。
- [x] 2.3 将 `../wo/prompts-template` 合入当前仓库。
- [x] 2.4 合并 `../wo/docs/specs` 和 `../wo/tests/specs` 中仍有效的长期规格和测试。
- [x] 2.5 合并根目录 shell 业务测试，并按当前仓库路径修正命令。

## 3. 修正模块路径和构建配置

- [x] 3.1 将 Go import 中的 `github.com/xbugs221/wo` 改为 `github.com/xbugs221/oz`。
- [x] 3.2 将 release ldflags 中的 `github.com/xbugs221/wo/internal/app.Version` 改为合并后的路径。
- [x] 3.3 将源码根定位逻辑从 `wo` module root 改为当前 `oz` module root。
- [x] 3.4 合并 `go.mod/go.sum`，确认 Go 版本选择并运行 `go mod tidy`。

## 4. 统一 acceptance 校验

- [x] 4.1 抽出当前 `wo` 允许的 acceptance 结构和校验逻辑，供 `cmd/oz` 与 `internal/app` 复用。
- [x] 4.2 更新 `oz validate`，要求 active change 包含合法 `acceptance.json`。
- [x] 4.3 更新 `oz status`，在 artifacts 中展示 `acceptance.json` 状态。
- [x] 4.4 更新 `oz` 单测，覆盖缺失、未知字段、无效 source/kind、有效当前格式。
- [x] 4.5 同步 `oz-create` skill 文案，避免创建当前 `wo` 不接受的 acceptance 字段。

## 5. 统一 CI、Release 和 update

- [x] 5.1 更新 CI：从当前 checkout 构建 `oz` 并放入 PATH，再运行 Go 测试和 shell 业务测试。
- [x] 5.2 更新 Release：同一 tag 构建并发布 `oz` 和 `wo` 两个 CLI。
- [x] 5.3 更新 `wo update`：从同一 release 批次检查、下载和校验两个二进制。
- [x] 5.4 更新 update 相关测试，覆盖不存在外部 latest `oz` 依赖的路径。

## 6. 文档和验证

- [x] 6.1 更新 README，说明单仓库包含 `oz` 规范 CLI 和 `wo` 执行器 CLI。
- [x] 6.2 明确旧 `wo` 运行态不会按仓库路径自动迁移。
- [x] 6.3 运行 `go test ./...`。
- [x] 6.4 运行根目录 shell 业务测试。
- [x] 6.5 运行本提案 `docs/changes/archive/2026-06-09-2-合并-wo-执行器到-oz-仓库/tests/` 下两个契约测试。
- [x] 6.6 运行 `go run ./cmd/oz validate 2-合并-wo-执行器到-oz-仓库 --json`。

## 历史测试更新

- `docs/changes/archive/2026-06-09-2-合并-wo-执行器到-oz-仓库/tests/2026-05-15-20-修复-cicd-并强化测试门禁-test_release_workflow_runs_business_tests.sh` 原本要求 CI/Release 下载外部 latest `oz`；这与本提案“必须使用当前 checkout 构建本地 `oz`”冲突，已改为断言 workflow 本地构建 `./cmd/oz` 和 `./cmd/wo`，并拒绝外部 latest 下载。
