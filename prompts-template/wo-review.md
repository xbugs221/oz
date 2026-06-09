读取:

- `{{.StatePath}}`
- `{{.AcceptancePath}}`

审核：

- 当前 git 工作区相对 `{{.BaselineHead}}` 的完整变更
- 对应 oz change：`{{.ChangePath}}/`
- `{{.AcceptancePath}}` 中的测试和证据合同
- tasks 是否全部完成且与实现一致
- 验收测试、实现代码、文档和 CLI 行为是否满足 proposal/design/spec
{{if .HasPlanningContext}}
- 规划并行上下文 artifact：`{{.PlanningContextPath}}`
{{end}}
{{if .HasParallelContext}}
- 执行前并行上下文 artifact：`{{.ParallelContextPath}}`
{{end}}
{{if .HasParallelReview}}
- 并行审核输入 artifact：`{{.ParallelReviewPath}}`
{{end}}

{{if .IsFirstRoleTurn}}
严格 JSON 要求：

- 第 2 轮及之后必须先验证上一轮 findings 是否已解决，再重新审核完整 diff
- 作为一个评审专家，提出问题，并分析根因，如有必要，应先行探索并提供证据，但不要修改源码
{{if .HasParallelReview}}
- 如 `{{.ParallelReviewPath}}` 已存在，必须把其中多视角 findings/evidence 汇总进最终 `{{.ReviewPath}}`。
- 如尚不存在该 artifact，先按 `workflow_config.parallel.groups.review` 的职责并行审核，并写入 `{{.ParallelReviewPath}}`。
- tool/subagent 只作为提示词角色线索，不作为 CLI 参数；当前主 agent 负责按成员 `name/purpose/stage/tool/subagent` 组织产物。
- 任一 gate_input 成员报告 blocker/major finding 时，最终 review `decision` 不得为 `clean`。
{{end}}

{{if .HasPreviousReview}}
上一轮审核修复循环产物：`{{.LatestPreviousReviewPath}}`

历史 review 数量：{{.PreviousReviewCount}}

{{if .HasPreviousFixSummary}}
Fix summary: `{{.LatestPreviousFixSummaryPath}}`
{{end}}

旧历史只在最新 artifact 引用旧 finding、同类 finding 重复出现、证据矛盾或升级修复原因不清时按需追溯；不要默认读取全部历史 review/fix artifact。
{{end}}

要求：

- 只输出一个 JSON 对象，不得输出 Markdown、注释、代码块或任何 JSON 以外文本。
- 不得包含额外字段，不得返回多个 JSON 值。
- `decision` 只允许 `clean` 或 `needs_fix`。
- `summary` / `findings[].title` / `findings[].evidence` / `findings[].recommendation` 必须可复核。
- evidence 必须可复核，不能只写结论。
- `clean` 的 evidence 必须引用验证命令 artifact，并引用截图、trace、QA、浏览器控制台、网络或等价运行时证据；Web app 不得只写“代码已检查”。
- `clean` 时 `findings` 为空、全部 `checks` 为 true、`evidence` 非空。
- `needs_fix` 时 `findings` 至少一项。
- `severity` 仅允许 `blocker`、`major`、`minor`（内部会归一化 `high/medium/low/nit/critical`）。
- 第 2 轮及之后 `clean` 时 `previous_findings_resolved` 必须为 true。
- 第 2 轮及之后，如果连续两轮没有实质变化，且最新 fix summary 明确表达无法完成或缺少继续修复条件，可以设置 `workflow_failure.failed: true` 和 `workflow_failure.reason`，让工作流直接失败停下。

- 编写前可先本地复测：`wo validate-review --artifact {{.ReviewPath}} --json`，不符合约束时先修正再提交。

写入：

```text
{{.ReviewPath}}
```

JSON schema：

```json
{
  "summary": "一句话总结审核结果",
  "decision": "clean",
  "findings": [],
  "checks": {
    "oz_aligned": true,
    "tasks_verified": true,
    "tests_meaningful": true,
    "implementation_scoped": true,
    "runtime_behavior_verified": true,
    "previous_findings_resolved": true
  },
  "evidence": [
    "validation artifact passed: <run>/validation-execution-1.json",
    "runtime evidence: Playwright trace/screenshots captured under test-results/<case>/"
  ]
}
```

如需修复，使用：

```json
{
  "summary": "一句话总结审核结果",
  "decision": "needs_fix",
  "findings": [
    {
      "title": "问题标题",
      "severity": "blocker",
      "evidence": "可复核证据",
      "recommendation": "问题根因，以及明确的修复建议"
    }
  ],
  "evidence": [],
  "workflow_failure": null,
  "checks": {
    "oz_aligned": false,
    "tasks_verified": false,
    "tests_meaningful": false,
    "implementation_scoped": true,
    "runtime_behavior_verified": false,
    "previous_findings_resolved": false
  }
}
```

如需提前终止无效循环，使用：

```json
{
  "summary": "连续两轮修复没有实质变化，且执行智能体报告无法继续可靠修复",
  "decision": "needs_fix",
  "workflow_failure": {
    "failed": true,
    "reason": "连续两轮 diff 没有实质变化；fix summary 表示缺少必要凭据，无法完成当前 findings"
  },
  "findings": [
    {
      "title": "问题标题",
      "severity": "blocker",
      "evidence": "可复核证据，例如两轮 diff/summary 对比和当前失败行为",
      "recommendation": "说明无法继续自动修复的根因"
    }
  ],
  "evidence": [
    "连续两轮 review 指向同类 finding",
    "最新 fix summary 报告无法完成",
    "git diff 对比显示没有实质修复"
  ],
  "checks": {
    "oz_aligned": false,
    "tasks_verified": false,
    "tests_meaningful": false,
    "implementation_scoped": true,
    "runtime_behavior_verified": false,
    "previous_findings_resolved": false
  }
}
```

如果发现任何字段缺失或格式问题，请先修复后重写；不要解释原因，不要拆分输出。
{{else}}
续轮要求：

- 复用当前角色会话：`{{.RoleSessionKey}}`
- 读取本轮目标：`{{.ReviewPath}}`
{{if .HasPreviousReview}}
- 核对上一轮审核：`{{.LatestPreviousReviewPath}}`
{{end}}
{{if .HasPreviousFixSummary}}
- 核对上一轮修复摘要：`{{.LatestPreviousFixSummaryPath}}`
{{end}}
{{if .HasParallelReview}}
- 汇总本轮并行审核输入：`{{.ParallelReviewPath}}`
{{end}}
- 只输出一个 JSON 对象，schema 与首轮 review artifact 相同；发现字段缺失或格式问题时直接重写 `{{.ReviewPath}}`。
{{end}}
