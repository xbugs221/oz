# 规格

## 验收矩阵

| 需求 | 场景 | 契约测试 | 证据 |
| --- | --- | --- | --- |
| 单仓库双 CLI | 同一 checkout 构建 oz 和 wo | `monorepo-cli-release-contract` | `monorepo-cli-release-log` |
| 单仓库双 CLI | CI/Release 使用本地 oz 而不是外部 latest | `monorepo-cli-release-contract` | `monorepo-cli-release-log` |
| acceptance 合同统一 | oz validate 接受当前 wo 格式 | `oz-acceptance-validation-contract` | `oz-acceptance-validation-log` |
| acceptance 合同统一 | oz validate 拒绝缺失或未知字段 | `oz-acceptance-validation-contract` | `oz-acceptance-validation-log` |
| 兼容执行器边界 | wo 继续通过 oz JSON 协议选择和校验 change | `monorepo-cli-release-contract` | `monorepo-cli-release-log` |

### 需求：单仓库双 CLI

系统必须在当前 `oz` 仓库中同时维护 `oz` 规范 CLI 和 `wo` 工作流执行器 CLI，并从同一个 checkout 构建、测试和发布。

#### 场景：同一 checkout 构建 oz 和 wo

- **给定** 开发者在合并后的仓库根目录
- **当** 运行 `bash docs/changes/2-合并-wo-执行器到-oz-仓库/tests/test_monorepo_cli_release_contract.sh`
- **则** 测试必须能从 `./cmd/oz` 构建 `oz`
- **并且** 测试必须能从 `./cmd/wo` 构建 `wo`
- **并且** `go list -m` 仍返回 `github.com/xbugs221/oz`
- **并且** `./cmd/wo` 的依赖中不得出现旧模块路径 `github.com/xbugs221/wo`
- **真实数据来源**：当前仓库的 Go module、`cmd/oz`、合入后的 `cmd/wo`、`internal/app`
- **入口路径**：仓库根目录的 shell 契约测试
- **关键断言**：两个二进制来自同一个源码 checkout，且 `wo` 不再依赖旧 module path
- **剩余风险**：该测试不验证用户状态目录迁移，迁移说明由 README 覆盖

#### 场景：CI 和 Release 使用本地 oz

- **给定** 合并后的仓库包含 GitHub Actions workflow
- **当** 运行 `bash docs/changes/2-合并-wo-执行器到-oz-仓库/tests/test_monorepo_cli_release_contract.sh`
- **则** workflow 中不得继续下载 `github.com/xbugs221/oz/releases/latest`
- **并且** workflow 必须包含从当前 checkout 构建 `./cmd/oz` 和 `./cmd/wo` 的步骤或命令
- **并且** workflow 必须继续运行 `go test ./...`
- **真实数据来源**：`.github/workflows/*.yml` 和 `.github/workflows/*.yaml`
- **入口路径**：仓库根目录的 shell 契约测试
- **关键断言**：CI/Release 不再把外部 latest `oz` 当作 `wo` 测试前置条件
- **剩余风险**：该测试不绑定具体资产命名，避免在实现期过早冻结发布包格式

### 需求：acceptance 合同统一

系统必须把当前 `wo` 已允许的 `acceptance.json` 格式作为 `oz` 的正式提案契约，并由 `oz validate` 在 sealed run 之前发现格式错误。

#### 场景：oz validate 接受当前 wo 格式

- **给定** 一个 active change 包含 `proposal.md`、`design.md`、`spec.md`、`task.md`、`tests/` 和当前 `wo` 允许的 `acceptance.json`
- **当** 运行 `bash docs/changes/2-合并-wo-执行器到-oz-仓库/tests/test_oz_acceptance_validation_contract.sh`
- **则** `oz validate <change> --json` 必须成功
- **并且** `acceptance.json` 只需要包含 `summary`、`required_tests`、`required_evidence` 及其当前 `wo` 已支持的子字段
- **真实数据来源**：契约测试创建的临时 oz 项目和真实 `oz` 二进制
- **入口路径**：仓库根目录的 shell 契约测试
- **关键断言**：`oz` 和 `wo` 对同一份当前格式 acceptance 合同达成一致
- **剩余风险**：该测试不验证 QA artifact 的 `acceptance_matrix`，那属于 `wo` 既有门禁

#### 场景：oz validate 拒绝缺失或未知字段

- **给定** 一个 active change 缺少 `acceptance.json`
- **当** 运行 `bash docs/changes/2-合并-wo-执行器到-oz-仓库/tests/test_oz_acceptance_validation_contract.sh`
- **则** `oz validate <change> --json` 必须失败并指出 acceptance 合同问题
- **给定** 一个 active change 的 `acceptance.json` 包含当前 `wo` schema 不允许的字段，例如 `coverage`
- **当** 再次运行校验
- **则** `oz validate <change> --json` 必须失败
- **真实数据来源**：契约测试创建的临时 oz 项目和真实 `oz` 二进制
- **入口路径**：仓库根目录的 shell 契约测试
- **关键断言**：`oz validate` 不再允许提案缺少 sealed run 必需的验收合同，也不接受 `wo` 会拒绝的扩展字段
- **剩余风险**：错误文案只要求包含 acceptance 相关信息，不绑定完整中文句子

### 需求：兼容执行器边界

系统必须在合并仓库后保留 `wo` 对 `oz list/status/validate/archive` JSON 协议的依赖边界，避免一次性重写 sealed run 主流程。

#### 场景：wo 继续通过 oz JSON 协议选择和校验 change

- **给定** `wo` 代码已经合入当前仓库
- **当** 运行 `bash docs/changes/2-合并-wo-执行器到-oz-仓库/tests/test_monorepo_cli_release_contract.sh`
- **则** `wo` 必须仍保留对 `oz list/status/validate/archive` 命令协议的调用能力
- **并且** 合并不得要求执行器测试改成直接 import `cmd/oz`
- **真实数据来源**：合入后的 `internal/app/change.go` 和既有 `wo` 测试中的 fake oz 契约
- **入口路径**：仓库根目录的 shell 契约测试及后续 `go test ./...`
- **关键断言**：合并改变发布和源码边界，不改变 sealed run 的规范消费协议
- **剩余风险**：未来可再通过独立提案评估是否把进程调用提取为共享包调用
