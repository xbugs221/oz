# 规格

## 验收矩阵

| 需求 | 场景 | 测试 | 证据 | 风险 |
| --- | --- | --- | --- | --- |
| 历史债务非阻断记录 | clean review 可以记录 out-of-scope 历史债务 | `review-non-blocking-debt` | `review-non-blocking-debt-log` | 只验证 review artifact，执行阶段需同步 QA schema |
| parallel gate 按 scope 阻断 | out-of-scope severe finding 不阻断 clean | `parallel-scope-gate` | `parallel-scope-gate-log` | 成员失败仍然阻断，不在本场景放宽 |
| parallel gate 按 scope 阻断 | 当前变更和旧格式 severe finding 仍阻断 clean | `parallel-scope-gate` | `parallel-scope-gate-log` | 缺省 scope 选择保守策略，避免旧 artifact 被误放行 |
| QA 验收矩阵保持严格 | QA 可记录历史债务但 acceptance_matrix 不得新增无关 id | `qa-acceptance-scope` | `qa-acceptance-scope-log` | 不覆盖浏览器截图，只覆盖 QA artifact gate 逻辑 |
| 旧提案兼容 | 未运行旧提案不增加新必填字段 | `legacy-active-change-compatibility` | `legacy-active-change-compatibility-log` | 只验证 oz validate，不启动完整 sealed run |
| Prompt 范围合同 | review/QA prompt 明确要求 scope 分类 | `prompt-scope-contract` | `prompt-scope-contract-log` | 文案可调整，但必须保留关键机器字段和边界规则 |

### 需求：历史债务非阻断记录

系统必须允许 clean review 记录不属于当前提案范围的历史债务，同时保持 `findings` 为空。

#### 场景：clean review 可以记录 out-of-scope 历史债务

- **给定** review artifact 的 `decision` 为 `clean`
- **并且** `findings` 为空、所有 checks 为 true、evidence 包含验证和运行时证据
- **并且** `non_blocking_findings` 中存在 `scope=out_of_scope_existing` 的 major finding
- **当** workflow 读取并验证该 review artifact
- **则** artifact 必须通过 review schema 校验
- **并且**该历史债务不得触发 fix 轮次
- **测试**：`docs/changes/archive/2026-06-10-12-收窄验收gate到提案范围/tests/test_review_non_blocking_debt_contract.sh`
- **真实数据来源**：脚本在 `internal/app` 包内临时写入 Go 测试，调用真实 `ReadReview`、`ValidateReview` 和 `NeedsFix`
- **入口路径**：`internal/app/review.go`
- **关键断言**：clean review 接受 `non_blocking_findings`；blocking `findings` 仍不允许出现在 clean review
- **剩余风险**：该测试不评价历史债务内容质量，只验证机器合同边界

### 需求：parallel gate 按 scope 阻断

系统必须只让当前提案范围内的 severe finding 阻断 clean review/QA。

#### 场景：out-of-scope severe finding 不阻断 clean

- **给定** `parallel-review-1.json` 中所有配置成员均成功
- **并且**某成员报告 `severity=major`、`scope=out_of_scope_existing` 的历史债务
- **当**主 review artifact 为 clean
- **则** `ValidateParallelReviewGate` 必须允许 workflow 继续
- **测试**：`docs/changes/archive/2026-06-10-12-收窄验收gate到提案范围/tests/test_parallel_scope_gate_contract.sh`
- **真实数据来源**：脚本临时写入真实 parallel review artifact JSON，调用真实 `ValidateParallelReviewGate`
- **入口路径**：`internal/app/parallel.go`
- **关键断言**：out-of-scope major finding 不阻断；成员 status 全部 success
- **剩余风险**：成员失败仍按既有硬阻断处理

#### 场景：当前变更和旧格式 severe finding 仍阻断 clean

- **给定** `parallel-review-1.json` 中存在 `scope=current_change` 的 major finding
- **当**主 review artifact 为 clean
- **则** gate 必须拒绝 clean
- **并且**缺少 `scope` 的旧格式 major finding 也必须继续拒绝 clean
- **测试**：`docs/changes/archive/2026-06-10-12-收窄验收gate到提案范围/tests/test_parallel_scope_gate_contract.sh`
- **真实数据来源**：同一脚本构造 current-change artifact 和 legacy artifact
- **入口路径**：`internal/app/parallel.go`
- **关键断言**：当前变更 severe finding 阻断；legacy missing scope 保持旧阻断行为
- **剩余风险**：不会自动判断真实 git blame，scope 仍由 reviewer/QA 给出并接受 review

### 需求：QA 验收矩阵保持严格

系统必须允许 QA 记录非阻断历史债务，但不得把历史债务混入当前 `acceptance_matrix`。

#### 场景：QA 可记录历史债务但 acceptance_matrix 不得新增无关 id

- **给定** `acceptance.json` 只定义 `contract-demo` 和 `runtime-demo`
- **并且** clean QA artifact 的 `acceptance_matrix` 逐项覆盖这两个 id
- **并且** QA artifact 通过 `non_blocking_findings` 记录 `scope=out_of_scope_existing` 的历史债务
- **当** workflow 调用 `ValidateQAAgainstAcceptance`
- **则** QA 必须通过
- **并且**如果 `acceptance_matrix` 额外引用历史债务 id，必须继续失败
- **测试**：`docs/changes/archive/2026-06-10-12-收窄验收gate到提案范围/tests/test_qa_acceptance_scope_contract.sh`
- **真实数据来源**：脚本临时写入 QA artifact JSON 并构造真实 acceptance contract
- **入口路径**：`internal/app/qa.go`
- **关键断言**：non-blocking finding 不影响 acceptance matrix；未知 acceptance id 仍失败
- **剩余风险**：该测试不启动浏览器，浏览器路径由具体业务提案自行定义

### 需求：旧提案兼容

系统必须保证执行本变更后，已创建但尚未运行的旧提案不需要补 scope 字段或迁移 acceptance 合同。

#### 场景：未运行旧提案不增加新必填字段

- **给定** 一个只包含既有 `brief.md`、`proposal.md`、`design.md`、`spec.md`、`task.md`、`acceptance.json` 和 `docs/changes/archive/2026-06-10-12-收窄验收gate到提案范围/tests/` 的 active change
- **并且**其 `acceptance.json` 不包含任何 scope 或 non-blocking 字段
- **当**用户运行 `oz validate <change> --json`
- **则**校验必须成功
- **测试**：`docs/changes/archive/2026-06-10-12-收窄验收gate到提案范围/tests/test_legacy_active_change_compatibility_contract.sh`
- **真实数据来源**：脚本创建临时 git 仓库和旧格式 active change，并运行真实编译后的 `oz validate`
- **入口路径**：`cmd/oz validate`
- **关键断言**：旧格式 active change 通过 validate；新 scope 字段不是 acceptance 必填项
- **剩余风险**：该测试不启动完整 `wo run`，因为兼容性重点是未运行提案的创建阶段合同

### 需求：Prompt 范围合同

review 和 QA prompt 必须把 scope 分类作为执行规则写清楚，避免 agent 只靠自由发挥判断范围。

#### 场景：review/QA prompt 明确要求 scope 分类

- **给定** 默认 prompt 模板
- **当**执行器生成 review 或 QA 指令
- **则** prompt 必须包含 `non_blocking_findings`
- **并且**必须包含 `out_of_scope_existing`
- **并且**必须说明当前变更、acceptance 合同和 introduced regression 仍是 hard block
- **测试**：`docs/changes/archive/2026-06-10-12-收窄验收gate到提案范围/tests/test_prompt_scope_contract.sh`
- **真实数据来源**：脚本读取仓库内真实 `prompts-template/wo-review.md` 和 `prompts-template/wo-qa.md`
- **入口路径**：默认 prompt 模板
- **关键断言**：review/QA prompt 都包含 scope 字段、非阻断 finding 字段和当前范围硬阻断说明
- **剩余风险**：该测试只验证 prompt 明确性，最终 gate 行为由前面的 Go 合同测试证明
