# 设计

## 总体方案

把当前仓库作为唯一源码仓库，保留模块名 `github.com/xbugs221/oz`，在其中同时维护：

- `cmd/oz`：规范、skill 安装、提案发现、校验和归档接口。
- `cmd/wo`：智能体工作流执行器。
- `internal/app`：从 `../wo` 合入的执行器业务逻辑。
- `prompts-template`：从 `../wo` 合入的内置提示词模板。
- `skills`：当前 `oz` 内置 skill。
- `docs/specs` 和 `docs/changes/archive/2026-06-09-2-合并-wo-执行器到-oz-仓库/tests/specs`：按业务能力合并 `oz` 与 `wo` 的长期规格和测试。

这样 `oz` 与 `wo` 仍是两个 CLI，但它们来自同一个 commit、同一个 Go module、同一个 release 批次。

## 关键决策

### 保留命令协议边界

`wo` 当前通过外部 `oz` 命令调用 `list/status/validate/archive`。本次合并先保留这个边界，避免把 `wo` 的 sealed run 状态机和 `oz` 的规范 CLI 一次性耦合。实现后 `wo` 测试中已有的 fake `oz` 契约仍应继续表达边界行为。

后续如果需要减少进程调用，可以再把 `oz` 规范逻辑提取成共享包，但这不是本次变更的必要条件。

### 统一 acceptance 契约

当前 `wo` 的 `ReadAcceptance` 使用严格 JSON 解码，实际允许字段只有：

- `summary`
- `required_tests`
- `required_evidence`

其中 `required_tests` 只接受 `id/source/path/command/purpose`，`required_evidence` 只接受 `id/kind/path/purpose`。本提案采用这个现有格式，不把 `coverage/assertions/expected_initial_failure` 写入 `acceptance.json`，否则 sealed run 会被当前 `wo` 拒绝。

实现时应把这套校验提升到 `oz validate`。推荐做法是抽出共享验收合同包，例如 `internal/acceptance`，让 `cmd/oz` 和 `internal/app` 使用同一份结构和校验函数。

### 发布和更新

Release workflow 应从同一个 checkout 构建两个二进制：

- `./cmd/oz`
- `./cmd/wo`

CI 不得再下载 GitHub latest `oz` 来测试 `wo`。测试需要 `oz` 时，应先本地构建当前 commit 的 `oz` 并放入 PATH。

`wo update` 的语义应从“分别追踪两个仓库 latest”调整为“从同一个 release 批次安装同版本 `wo` 和 `oz`”。如果继续保留独立工具结果展示，也必须保证 latest 来源一致，不能混用不同仓库的最新版本。

### Go 版本与依赖

`oz` 当前是 Go 1.22 无外部依赖，`wo` 当前是 Go 1.26.2 并依赖 `gopkg.in/yaml.v3`。执行阶段需要先验证 `wo` 是否能降到 Go 1.22；如果不能，应明确把合并后模块的 Go 版本提升到当前可验证版本，并同步 CI。

## 迁移步骤

1. 从 `../wo` 合入 `cmd/wo`、`internal/app`、`prompts-template`、`.github/workflows` 中仍有效的测试门禁、长期规格和业务测试。
2. 将 import、ldflags 和源码根定位中的 `github.com/xbugs221/wo` 改为 `github.com/xbugs221/oz`。
3. 保留 `cmd/oz` 的现有命令面，并补齐 `acceptance.json` 校验。
4. 合并 `go.mod/go.sum`，选择并验证 Go 版本。
5. 改造 CI/Release，让本地构建的 `oz` 参与 `wo` 测试，并在同一 release 发布两个 CLI。
6. 更新 README，把项目定位改为“`oz` 规范工具 + `wo` 工作流执行器”的单仓库组合。

## 风险和处理

- **旧运行态路径变化**：`wo` 状态目录的 repo key 来自仓库绝对路径，合并后旧 run/batch 不会自动出现。README 需要提醒用户迁移前处理未完成任务。
- **测试数量增加**：`wo` 的 shell 业务测试较多，CI 时间会上升。先保持真实业务测试门禁，再按实际耗时决定是否分组。
- **Release 资产兼容**：旧 `oz` 与 `wo` 资产命名不同。执行阶段需要选择统一命名，并同步 `wo update` 的资产选择逻辑和测试。
- **acceptance 格式认知差异**：skill 文档中的扩展字段暂时不能写入 `acceptance.json`，否则当前 `wo` 不接受。后续如要扩展字段，应先升级 `wo` schema，再更新 `oz-create`。
