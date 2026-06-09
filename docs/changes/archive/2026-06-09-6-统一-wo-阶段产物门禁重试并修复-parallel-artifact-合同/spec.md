# 规格

## 验收矩阵

| 场景 | required_tests | required_evidence |
| --- | --- | --- |
| 所有主阶段产物缺失或非法都会同会话重试 | `contract-stage-artifact-gate-retry-all-roles` | `runtime-stage-artifact-gate-retry-all-roles` |
| batch 中 execution 产物修复后继续后续 change | `contract-batch-continues-after-stage-artifact-repair` | `runtime-batch-continues-after-stage-artifact-repair` |
| parallel subagent 的 info severity 不会中断 workflow | `contract-parallel-subagent-info-severity` | `runtime-parallel-subagent-info-severity` |
| 所有阶段首轮提示词保留完整阶段合同 | `contract-stage-prompt-completeness` | `runtime-stage-prompt-completeness` |
| 全仓库 Go 回归保持通过 | `root-go-test-regression` | `runtime-root-go-tests` |

### 需求：主阶段 artifact gate 缺失或非法时同会话重试

系统必须在任意主阶段 agent 返回后检查该阶段应有产物。产物缺失、格式非法或合同不满足时，系统必须记录 `Stage artifact gate failed`，resume 同一角色 session，要求只补写或改写当前阶段产物，最多重试 3 次。第三次仍失败后才进入阻断状态。

#### 场景：所有主阶段产物缺失或非法都会同会话重试

- **测试文件**：`docs/changes/6-统一-wo-阶段产物门禁重试并修复-parallel-artifact-合同/tests/test_stage_artifact_gate_retry_all_roles.sh`
- **真实数据来源**：测试构造临时 git 仓库和真实 oz change，使用源码构建出的真实 `wo` 二进制运行默认 `go-dag`，fake `codex` 首次故意漏写或写坏 execution、review、fix、QA、archive 产物，第二次在同角色 session 中修正。
- **入口路径**：`wo run --change 1-stage-artifact-retry --json`
- **关键断言**：
  - execution 首次未完成 task 不得直接 failed，第二次必须用原 executor session 收到 `Stage artifact gate failed` 后完成 task。
  - review 首次缺失或非法 `review-N.json` 后必须用原 reviewer session 重写。
  - fix 首次缺失 `fix-N-summary.md` 后必须用原 fixer session 补写。
  - QA 首次 acceptance matrix 不完整后必须用原 QA session 重写。
  - archive 首次缺少 delivery summary 或归档目录后必须用原 archiver session 补写。
  - 最终 run 状态为 `done`，并保留每次 artifact gate 失败的验证 artifact。
- **剩余风险**：该测试使用 fake `codex` 稳定复现产物缺失和修正，不依赖真实 LLM 是否遵守提示词。

### 需求：batch 不因可修复阶段产物问题中断

系统必须让 batch worker 等待当前 run 的 artifact gate retry 结果。当前 change 的阶段产物在同会话修复后，batch 必须继续执行后续 change；只有达到重试上限或真实 agent/backend 失败时才停止 batch。

#### 场景：batch 中 execution 产物修复后继续后续 change

- **测试文件**：`docs/changes/6-统一-wo-阶段产物门禁重试并修复-parallel-artifact-合同/tests/test_batch_continues_after_stage_artifact_repair.sh`
- **真实数据来源**：测试手工创建一个包含两个 active changes 的 batch state，真实 `wo batch --batch-id ... --json` 执行队列；fake `codex` 让第一个 change 的 execution 首次不完成 task，第二次修复，第二个 change 正常通过。
- **入口路径**：`wo batch --batch-id batch-artifact-retry --json`
- **关键断言**：
  - 第一个 change 不因 `execution 阶段 artifact 未完成` 进入 batch failed。
  - 第一个 change 的 execution 第二次使用原 executor session。
  - batch state 最终为 `done`。
  - batch 创建并完成两个 run，第二个 change 的 task 也被执行。
- **剩余风险**：该测试覆盖 batch worker 与 run state 交互，不覆盖交互式菜单创建 batch 的输入解析；输入解析已有既有测试覆盖。

### 需求：parallel subagent member artifact 合同统一归一化

系统必须在 subagent member artifact 写出后立即执行统一 parse、normalize、validate。字段类型错误、未知字段、不可归一的 severity 或成员集合不匹配必须在 subagent 局部 retry；可归一的 severity 别名必须写回规范化 artifact。fan-in 只能汇总已经通过该边界的 member artifact。

#### 场景：parallel subagent 的 info severity 不会中断 workflow

- **测试文件**：`docs/changes/6-统一-wo-阶段产物门禁重试并修复-parallel-artifact-合同/tests/test_parallel_subagent_info_severity_contract.sh`
- **真实数据来源**：测试在临时仓库启用 `planning_context` parallel group，fake `pi` 为真实 `SUBAGENT_OUTPUT` 写入 `findings[].severity: "info"`，真实 `wo` 二进制执行 `go-dag` fan-out/fan-in。
- **入口路径**：`wo run --change 1-parallel-info-severity --json`
- **关键断言**：
  - workflow 不因 `severity 无效："info"` failed。
  - member artifact 和 `parallel-planning-context.json` 中的 severity 最终写回为 `minor`。
  - fan-in 继续执行，execution 和 archive 阶段完成。
  - 如果实现选择先 retry 而不是直接归一化，retry 必须使用同一 `pi` subagent session，且提示只重写 `SUBAGENT_OUTPUT`。
- **剩余风险**：该测试覆盖 `info` 这类常见提示性口径；执行阶段可继续补充单元测试覆盖其他 alias。

### 需求：所有阶段首轮提示词保留完整阶段合同

系统必须保证内置阶段模板在首轮 agent 调用时包含当前阶段完成工作所需的最小完整合同。execution/write 阶段不得退化为只读取 `state.json` 并调用技能；review、QA、fix、archive 必须保留输入 artifact、目标输出 artifact、禁止事项、验收边界和证据要求。只有同一角色续轮可以省略重复 JSON 示例或方法论说明，但不得省略当前目标 artifact、role session、上一轮必要引用或结构化输出要求。

#### 场景：渲染后的内置阶段提示词满足首轮完整和续轮精简边界

- **测试文件**：`docs/changes/6-统一-wo-阶段产物门禁重试并修复-parallel-artifact-合同/tests/test_stage_prompt_contract_completeness.sh`
- **真实数据来源**：测试向 `internal/app` 注入临时 Go 测试，使用真实 `prompts-template/*.md`、真实 `DefaultWorkflowConfig()` 和真实 `promptContext` 渲染 `wo-discuss`、`wo-start`、`wo-review`、`wo-qa`、`wo-fix`、`wo-done`。
- **入口路径**：`bash docs/changes/6-统一-wo-阶段产物门禁重试并修复-parallel-artifact-合同/tests/test_stage_prompt_contract_completeness.sh`
- **关键断言**：
  - `wo-start` 首轮必须包含 `proposal.md`、`design.md`、`spec.md`、`task.md`、`acceptance.json`、`tests/`、`required_tests`、不得删除/弱化契约测试、`oz status` 和 `tasks.done` 完成标准。
  - review 首轮必须包含 baseline diff、change 文档、task/验收边界、`review-N.json`、严格 JSON 和示例；review 续轮必须保留 `review-N.json`、上一轮 review/fix 引用和 JSON 输出要求，但不重复 schema 示例。
  - QA 首轮必须包含 `review-N.json`、`acceptance.json`、`required_tests`、`required_evidence`、`acceptance_matrix`、`qa-N.json` 和不得修改源码/acceptance；QA 续轮必须保留 `qa-N.json` 和 acceptance matrix，但不重复示例。
  - fix 首轮必须包含当前 review/QA、只修当前 findings、根因分析、不得弱化 acceptance、验证要求和 `fix-N-summary.md`；fix 续轮必须保留当前 review/QA 和 summary 路径，但不重复方法论长文。
  - archive 必须包含 `oz-archive`、最新 review/QA clean 条件、acceptance matrix 覆盖、`delivery-summary.md`、归档目录和 commit 范围。
- **剩余风险**：该测试覆盖内置模板渲染合同，不替代真实 agent 是否遵守 prompt 的运行时验证；遵守问题由 artifact gate retry 继续兜底。

### 需求：阶段产物门禁改造后保持 Go 回归通过

系统必须在统一 artifact gate retry 和 parallel member normalize 后保持现有 Go 包测试通过，避免破坏状态机、status、restart、validation 或配置读取合同。

#### 场景：全仓库 Go 回归保持通过

- **测试文件**：无新增文件，复用根目录 Go 测试。
- **真实数据来源**：仓库真实 Go 包、当前测试 fixture 和 CLI 合同。
- **入口路径**：`go test ./...`
- **关键断言**：
  - 所有 Go 包测试通过。
  - 与新意图冲突的旧测试必须更新为同会话 artifact gate retry，而不是保留直接 failed 的预期。
- **剩余风险**：Go 回归不替代本提案 shell 契约测试。
