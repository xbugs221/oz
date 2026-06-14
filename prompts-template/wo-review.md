读取:

- `{{.StatePath}}`
- `{{.AcceptancePath}}`
- 当前完整变更：`{{.ChangePath}}/`
- 当前 diff baseline：`{{.BaselineHead}}`
{{if .HasPlanningContext}}
- `{{.PlanningContextPath}}`
{{end}}
{{if .HasParallelContext}}
- `{{.ParallelContextPath}}`
{{end}}
{{if .HasParallelReview}}
- `{{.ParallelReviewPath}}`
{{end}}

任务：

- 只审核当前提案范围，不修改源码。
- 当前提案问题写 `findings`；历史债务或无关问题写 `non_blocking_findings`。
- `required_evidence` 表示可复核运行证据，不表示产物必须版本化。不要要求 `test-results/`、截图、trace 或 runtime log 被 git 跟踪；若合同或测试用 `git ls-files --error-unmatch` 强制跟踪 `test-results`，应作为当前提案合同设计错误报告。
{{if .HasParallelReview}}
- 读或生成 `{{.ParallelReviewPath}}`；先把 gate_input 成员结论归一化：正向确认、无操作项、其它提案内容和历史债务不得进入 `findings`。
- 成员 artifact 若为 `relevant:false` 且 `findings=[]`，表示该职责与当前提案无关，不得按失败处理；可在 `evidence` 中记录其 `irrelevant_reason`。
- 只有你复核后确认属于当前提案 acceptance/spec 的可复现 blocker/major 行为失败，才写入最终 `findings` 并令 `decision=1`；更深覆盖建议、可维护性建议或未承诺扩展写入 `non_blocking_findings` 或 `evidence`。
{{end}}

{{if .HasPreviousReview}}
上一轮审核：`{{.LatestPreviousReviewPath}}`
历史 review 数量：{{.PreviousReviewCount}}
{{if .HasPreviousFixSummary}}
Fix summary: `{{.LatestPreviousFixSummaryPath}}`
{{end}}
{{end}}

写入：

```text
{{.ReviewPath}}
```

{{if .IsFirstRoleTurn}}
严格 JSON：只写一个 JSON object，字段规则：

- `summary`
- `decision`: 0=clean, 1=needs_fix
- `evidence[]`: 字符串数组；每一项必须是字符串，不能是对象或数组；内容必须可复核，写命令、artifact、截图、trace、QA、控制台或网络证据
- `findings[]`: `{title,severity,scope,evidence,recommendation}`
- `severity`: 1=blocker, 2=major, 3=minor
- `scope`: 1=current_change, 2=introduced_regression, 0=out_of_scope_existing
- `checks`: `oz_aligned,tasks_verified,tests_meaningful,implementation_scoped,runtime_behavior_verified,previous_findings_resolved`
- `non_blocking_findings[]`: 仅 scope=0
- `workflow_failure`: 连续无实质修复且无法继续时使用

clean 要求：`decision=0`、`findings=[]`、`checks` 全 true、`evidence` 非空，并覆盖 acceptance_contract。
needs_fix 要求：`decision=1`、`findings` 至少一项。
{{else}}
续轮：

- 复用当前角色会话：`{{.RoleSessionKey}}`
- 读 `{{.ReviewPath}}`，按首轮同 schema 重写一个 JSON object。
{{if .HasPreviousReview}}
- 核对 `{{.LatestPreviousReviewPath}}`
{{end}}
{{if .HasPreviousFixSummary}}
- 核对 `{{.LatestPreviousFixSummaryPath}}`
{{end}}
{{end}}
