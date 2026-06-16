读取：`{{.StatePath}}`、`{{.AcceptancePath}}`、当前完整变更 `{{.ChangePath}}/`、当前 diff baseline `{{.BaselineHead}}`
{{if .HasPlanningContext}}可选上下文：`{{.PlanningContextPath}}`
{{end}}{{if .HasParallelContext}}可选上下文：`{{.ParallelContextPath}}`
{{end}}{{if .HasParallelReview}}review helper：`{{.ParallelReviewPath}}`
{{end}}

任务：

- 只审核当前提案范围，不修改源码。
- 当前提案问题写 `findings`；历史债务或无关问题写 `non_blocking_findings`，scope 用 `out_of_scope_existing`。
- blocking scope 只允许 `current_change` 或 `introduced_regression`；acceptance_contract 未满足必须可复现。
- `required_evidence` 只要求可复核，不要求 `test-results/`、截图、trace 或 runtime log 进 git；强制跟踪运行产物属于合同设计错误。
{{if .HasParallelReview}}- 读或生成 `{{.ParallelReviewPath}}`；先把 gate_input 成员结论归一化。relevant:false 且 findings=[] 的 helper 只作 evidence，不得阻断。只有复核确认当前 acceptance/spec 的 blocker/major 失败才进入最终 `findings`。
{{end}}
{{if .HasPreviousReview}}
上一轮：`{{.LatestPreviousReviewPath}}`；历史 review 数量：{{.PreviousReviewCount}}
{{if .HasPreviousFixSummary}}Fix summary：`{{.LatestPreviousFixSummaryPath}}`
{{end}}{{end}}

写入：`{{.ReviewPath}}`

写入后运行：`oz flow validate-review --artifact "{{.ReviewPath}}" --json`

{{if .IsFirstRoleTurn}}
严格 JSON：只写一个 JSON object。

字段：`summary`、`decision`(0=clean,1=needs_fix)、`evidence[]`、`findings[]`、`checks`、`non_blocking_findings[]`、`workflow_failure`。

findings 字段：`{title,severity,scope,evidence,recommendation}`；`severity`: 1=blocker, 2=major, 3=minor；`scope`: 1=current_change, 2=introduced_regression, 0=out_of_scope_existing。`evidence[]` 每项必须是字符串，写可复核命令、artifact、截图、trace、QA、控制台或网络证据。

clean：`decision=0`、`findings=[]`、`checks` 全 true、`evidence` 非空并覆盖 acceptance_contract。needs_fix：`decision=1` 且 `findings` 非空。
{{else}}
续轮：复用当前角色会话 `{{.RoleSessionKey}}`，按同 schema 重写 `{{.ReviewPath}}` 为一个 JSON object；核对上一轮 review/fix 后只报告仍存在的问题。
{{end}}
