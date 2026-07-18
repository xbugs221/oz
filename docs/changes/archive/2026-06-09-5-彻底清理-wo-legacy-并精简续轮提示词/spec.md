# 规格

## 验收矩阵

| 场景 | required_tests | required_evidence |
| --- | --- | --- |
| wo 运行时代码不再保留 Dagu 残留 | `contract-no-dagu-runtime-residue` | `runtime-no-dagu-runtime-residue` |
| wo node 入口随 Dagu 执行器移除 | `contract-no-dagu-runtime-residue` | `runtime-no-dagu-runtime-residue` |
| prompt 配置拒绝 writing 兼容键 | `contract-prompt-legacy-removed` | `runtime-prompt-legacy-removed` |
| sealed run 不再读取 legacy prompt 快照 | `contract-prompt-legacy-removed` | `runtime-prompt-legacy-removed` |
| review/fix 续轮提示词只保留增量上下文 | `contract-review-fix-resumed-prompt-compact` | `runtime-review-fix-resumed-prompt-compact` |
| 全仓库 Go 回归保持通过 | `root-go-test-regression` | `runtime-root-go-tests` |

### 需求：wo 工作流彻底移除 Dagu 运行时残留

系统必须只保留内嵌 `go-dag` 工作流。当前运行时代码、当前规格和长期测试不得继续维护 Dagu executor、Dagu YAML、Dagu CLI 调用或 Dagu 专属节点入口。

#### 场景：wo 运行时代码不再保留 Dagu 残留

- **测试文件**：`docs/changes/archive/2026-06-09-5-彻底清理-wo-legacy-并精简续轮提示词/tests/test_no_dagu_runtime_residue_contract.sh`
- **真实数据来源**：测试从源码构建真实 `wo` 二进制，并扫描当前运行时代码、prompt 模板、README、当前规格和长期测试文件。
- **入口路径**：`go build ./cmd/wo`、`wo graph --change demo --format json`、`wo graph --change demo --format mermaid`、源码扫描。
- **关键断言**：
  - `wo graph --format json` 和 `wo graph --format mermaid` 仍可运行。
  - 当前 runtime/spec/tests 目标路径不包含 `Dagu`、`dagu`、`StartDagu`、`ExportWorkflowDagu`、`runDagu`、`writeRunDagu` 等 Dagu 残留。
  - `README.md` 和当前 `docs/specs/codex-workflow-cli/spec.md` 不再把 Dagu 描述成当前合同的一部分。
  - 扫描不包含 `docs/changes/archive/`，历史归档材料不参与本次清理。
- **剩余风险**：该测试扫描当前维护面，不检查历史归档文档。

#### 场景：wo node 入口随 Dagu 执行器移除

- **测试文件**：`docs/changes/archive/2026-06-09-5-彻底清理-wo-legacy-并精简续轮提示词/tests/test_no_dagu_runtime_residue_contract.sh`
- **真实数据来源**：真实 `wo` 二进制在临时 git 仓库中执行隐藏 node 命令。
- **入口路径**：`wo node run-stage --run-id missing --stage execution --json`
- **关键断言**：
  - 命令返回非零。
  - 错误必须表现为未知命令或已移除命令。
  - 错误不得继续进入 run-node 状态读取或 Dagu node helper。
- **剩余风险**：该场景只约束 CLI 入口；`go-dag` 内部仍可通过 Go 函数调用 stage/gate/fanin helper。

### 需求：prompt legacy 兼容路径全部移除

系统必须只接受当前 prompt 和 stage key。`writing`、历史 `runs/<run-id>/prompts/*.md` 快照和只为兼容保留的 legacy role 不再作为恢复或配置路径。

#### 场景：prompt 配置拒绝 writing 兼容键

- **测试文件**：`docs/changes/archive/2026-06-09-5-彻底清理-wo-legacy-并精简续轮提示词/tests/test_prompt_legacy_removed_contract.sh`
- **真实数据来源**：注入到 `internal/app` 包内的 Go 契约测试使用真实 `LoadWorkflowConfig` 读取临时仓库 `wo.yaml`。
- **入口路径**：`LoadWorkflowConfig(repo)`。
- **关键断言**：
  - `wo.prompts.writing` 必须被拒绝。
  - `wo.workflow.stages.writing` 必须被拒绝。
  - 默认配置和 stage kind 列表不得包含 `writing`。
- **剩余风险**：该测试直接覆盖配置读取层，不单独覆盖人类 `wo config` 输出；默认配置已有根测试覆盖。

#### 场景：sealed run 不再读取 legacy prompt 快照

- **测试文件**：`docs/changes/archive/2026-06-09-5-彻底清理-wo-legacy-并精简续轮提示词/tests/test_prompt_legacy_removed_contract.sh`
- **真实数据来源**：注入 Go 契约测试在真实 run state 目录下写入历史 `prompts/wo-start.md` 和只含 `prompts.writing` 的 `prompt-snapshot.yaml`。
- **入口路径**：`promptForStage(repo, State{Sealed: true})`。
- **关键断言**：
  - 缺少 `prompt-snapshot.yaml` 时，即使存在 `runs/<run-id>/prompts/wo-start.md`，也必须失败关闭。
  - `prompt-snapshot.yaml` 只包含 `prompts.writing` 时，execution 和 fix 不得映射到该 prompt。
  - 错误必须指向缺少当前 prompt 快照，而不是回退当前配置或历史文件。
- **剩余风险**：旧 sealed run 不能继续恢复，这是本提案明确接受的破坏性清理。

### 需求：review/fix 续轮提示词只保留增量上下文

系统必须让 `review` 和 `fix` 角色首轮提示词保留完整方法论和示例；第 2 轮及之后依赖同一角色会话历史，只提供本轮新增路径、上一轮 findings、输出位置和必要格式边界。

#### 场景：review/fix 续轮提示词只保留增量上下文

- **测试文件**：`docs/changes/archive/2026-06-09-5-彻底清理-wo-legacy-并精简续轮提示词/tests/test_review_fix_resumed_prompt_compact_contract.sh`
- **真实数据来源**：注入到 `internal/app` 包内的 Go 契约测试读取真实 `prompts-template/wo-review.md` 和 `prompts-template/wo-fix.md` 并通过真实 `renderPromptTemplate` 渲染。
- **入口路径**：`renderPromptTemplate("wo-review", ...)`、`renderPromptTemplate("wo-fix", ...)`。
- **关键断言**：
  - `review_1` 包含完整 JSON schema、示例和审核方法论。
  - `review_2` 包含当前 `review-2.json`、上一轮 `review-1.json`、`fix-1-summary.md` 和角色会话 key，但不再重复首轮方法论、长 checklist、示例和 workflow failure 大段说明。
  - `fix_1` 包含根因分析方法论和 `fix-1-summary.md`。
  - `fix_2` 包含当前 `review-2.json`、`qa-2.json`、`fix-2-summary.md` 和角色会话 key，但不再重复首轮根因分析方法论、验收保护长句和通用历史说明。
- **剩余风险**：测试按关键文本片段验证精简效果，不对自然语言逐字冻结；执行阶段仍可调整措辞。

### 需求：清理后保持现有 Go 回归通过

系统必须在移除旧运行时和配置兼容后保持当前 Go 包测试通过，避免把清理变成状态机、配置合并或 artifact gate 回归。

#### 场景：全仓库 Go 回归保持通过

- **测试文件**：无新增文件，复用根目录 Go 测试。
- **真实数据来源**：仓库真实 Go 包、当前测试 fixture 和现有 CLI 合同。
- **入口路径**：`go test ./...`
- **关键断言**：
  - 所有 Go 包测试通过。
  - 与新意图冲突的旧测试必须更新为拒绝 legacy 行为，而不是删除覆盖。
- **剩余风险**：shell 规格测试由前述 change contract 覆盖，根 Go 测试不替代这些契约测试。
