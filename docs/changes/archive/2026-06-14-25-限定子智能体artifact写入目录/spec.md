# 规格

## 验收矩阵

| 需求 | 场景 | 测试 | 证据 |
| --- | --- | --- | --- |
| 独立 artifact 目录 | member artifact 路径固定到专属目录的 `member.json` | `contract-subagent-artifact-directory` | `subagent-artifact-directory-log` |
| prompt 文件交付合同 | subagent prompt 要求写文件并运行校验命令 | `contract-subagent-artifact-directory` | `subagent-artifact-directory-log` |
| member artifact CLI 校验 | 合法 artifact 可由 CLI 校验通过，非法字段给出可修正错误 | `contract-subagent-artifact-directory` | `subagent-artifact-directory-log` |
| helper 作为证据输入 | artifact 交付失败不直接阻断主流程 | `regression-helper-advisory-input` | `go-test-subagent-advisory-log` |

### 需求：独立 artifact 目录

系统必须为每个 parallel subagent member 分配独立 artifact 目录，并把对外 member artifact 路径固定为该目录下的 `member.json`。

#### 场景：member artifact 路径固定到专属目录的 `member.json`

- **对应测试**：`docs/changes/25-限定子智能体artifact写入目录/tests/test_subagent_artifact_directory_contract.sh`
- **真实数据来源**：测试直接调用 `internal/app` 的真实 `memberArtifactPath` 和真实 member slug 逻辑。
- **入口路径**：`bash docs/changes/25-限定子智能体artifact写入目录/tests/test_subagent_artifact_directory_contract.sh`
- **关键断言**：路径 basename 必须是 `member.json`；父目录必须以 `.artifact` 结尾；同一 group/iteration 下不同 member 不得共用目录。
- **剩余风险**：该测试不验证真实 agent 进程是否使用 OS sandbox，只验证 `wo` 对外分配的写入目标。

### 需求：prompt 文件交付合同

系统必须在 subagent prompt 中提供固定 artifact 路径和校验命令，使 agent 能先写文件、再自行校验，而不是依赖最终回复裸 JSON。

#### 场景：subagent prompt 要求写文件并运行校验命令

- **对应测试**：`docs/changes/25-限定子智能体artifact写入目录/tests/test_subagent_artifact_directory_contract.sh`
- **真实数据来源**：测试调用真实 `subagentPrompt`，使用 QA group 的真实 member 配置样例。
- **入口路径**：`bash docs/changes/25-限定子智能体artifact写入目录/tests/test_subagent_artifact_directory_contract.sh`
- **关键断言**：prompt 必须包含 `ARTIFACT_DIR`、`ARTIFACT_PATH`、目标路径、`wo validate-member-artifact` 命令，并不得继续要求“最终只输出一个 JSON object”作为主要交付方式。
- **剩余风险**：prompt 合同不能强制 agent 遵守；执行阶段仍需保留文件存在性和 schema gate。

### 需求：member artifact CLI 校验

系统必须提供可由 subagent 自行运行的 member artifact 校验命令，成功时输出明确通过信息，失败时给出字段级修复提示。

#### 场景：合法 artifact 可由 CLI 校验通过，非法字段给出可修正错误

- **对应测试**：`docs/changes/25-限定子智能体artifact写入目录/tests/test_subagent_artifact_directory_contract.sh`
- **真实数据来源**：测试创建真实 JSON artifact 文件，并通过 `app.Run` 调用真实 CLI 分发入口。
- **入口路径**：`bash docs/changes/25-限定子智能体artifact写入目录/tests/test_subagent_artifact_directory_contract.sh`
- **关键断言**：合法 artifact 返回成功并输出“member artifact 合法”；`evidence` 写成对象时返回错误，错误文本必须包含 `field=evidence`、`expected=array<string>` 和修复建议。
- **剩余风险**：测试覆盖 CLI 入口和错误信息，不覆盖所有 schema 字段排列组合。

### 需求：helper 作为证据输入

系统必须把 parallel helper 视为主阶段的证据输入，而不是让单个 helper 的 artifact 交付问题直接成为 hard gate；写边界破坏仍必须阻断。

#### 场景：artifact 交付失败不直接阻断主流程

- **对应测试**：`bash -o pipefail -c 'test -f internal/app/go_dag_execution_context_test.go && mkdir -p test-results/25-subagent-artifact-directory && go test ./internal/app -run "TestSubagentMalformedArtifactBecomesAdvisoryInput|TestSubagentBoundaryBlocksSiblingRunArtifact|TestSubagentBoundaryAllowsSessionProgressStateWrite" -count=1 2>&1 | tee test-results/25-subagent-artifact-directory/subagent-advisory-go-test.log'`
- **真实数据来源**：测试使用真实 `nodeRunSubagent`、真实默认 QA helper 配置、真实 member artifact 读写和真实 run state。
- **入口路径**：`bash -o pipefail -c 'test -f internal/app/go_dag_execution_context_test.go && mkdir -p test-results/25-subagent-artifact-directory && go test ./internal/app -run "TestSubagentMalformedArtifactBecomesAdvisoryInput|TestSubagentBoundaryBlocksSiblingRunArtifact|TestSubagentBoundaryAllowsSessionProgressStateWrite" -count=1 2>&1 | tee test-results/25-subagent-artifact-directory/subagent-advisory-go-test.log'`
- **关键断言**：QA required helper 连续输出不可捕获内容时，节点写出 `status:"failed"` 的 member artifact 并返回 completed，run state 不进入 failed；sibling artifact 写入仍触发只读边界失败；合法 session progress 写 `state.json` 不被误拦。
- **剩余风险**：helper delivery failure 会降低证据完整性，主 QA/Review 仍需在自身 artifact 中说明证据充分性。
