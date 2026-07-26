读取：`{{.StatePath}}`、`{{.AcceptancePath}}`、当前完整变更 `{{.ChangePath}}/`、当前 diff baseline `{{.BaselineHead}}`

任务：

- 只处理当前提案范围：检查实现、修正发现的问题并运行确定性验证。
- 每轮重新核对 state、acceptance、完整 diff 与验证结果；不得仅依赖会话记忆。
- 当前提案问题写 `findings`；历史债务写 `non_blocking_findings`。
{{if .HasPreviousRepair}}
- 上一轮检查点：`{{.LatestPreviousRepairPath}}`。
{{end}}
{{if .HasPreviousQA}}
- 触发本轮的上一轮独立 QA：`{{.LatestPreviousQAPath}}`；必须逐项复核并处理其中的 findings。
{{end}}

写入：`{{.RepairPath}}`

写入后运行：`oz flow validate-repair --artifact "{{.RepairPath}}" --json`

严格 JSON：字段为 `summary`、`decision`、`evidence[]`、`findings[]`、`checks`、`non_blocking_findings[]`。
`decision` 只能是 `clean` 或 `needs_more`。发现并修正问题、仍需下一轮确认时用 `needs_more`；确认当前范围无已知问题时用 `clean`。
每个 finding 使用 `{title,severity,scope,evidence,recommendation}`。

同一角色续轮必须复用 backend-scoped 会话 `{{.RoleSessionKey}}`；repairer 不能自行归档，clean 后仍须独立 QA 放行。
