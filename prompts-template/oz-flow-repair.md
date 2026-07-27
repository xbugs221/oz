读取：`{{.StatePath}}`、`{{.AcceptancePath}}`、当前完整变更 `{{.ChangePath}}/`、当前 diff baseline `{{.BaselineHead}}`

任务：

- 只处理当前提案范围：检查实现、修正发现的问题并运行确定性验证。
- `pre_qa_audit` 模式必须覆盖 acceptance、完整 diff、源码和测试；发现并修复问题后继续全量自查，只有一轮没有新问题且全部 required tests 通过才可移交 QA。
- `qa_targeted_repair` 模式只处理运行时上下文给出的最新 QA findings、失败验收项及直接相关回归；不得借机重新扩大为全量自查。
- 每轮重新核对 state、acceptance、完整 diff 与验证结果；不得仅依赖会话记忆。
- 移交前必须复跑失败测试、`acceptance.json.required_tests` 全集和配置中的 validation commands；任一失败或结果未绑定当前 diff 时不得声明完成。
- 当前提案问题写 `findings`；历史债务写 `non_blocking_findings`。
{{if .HasPreviousRepair}}
- 上一轮检查点：`{{.LatestPreviousRepairPath}}`。
{{end}}
{{if .HasPreviousQA}}
- 触发本轮的上一轮独立 QA：`{{.LatestPreviousQAPath}}`；必须逐项复核并处理其中的 findings。
{{end}}
{{if .IsRepairConfirmation}}
- 本轮是上一轮 `clean` 后的强制重审确认。必须重新从 state、acceptance、完整 diff 和验证结果开始审查，不得直接复述上一轮结论。
- 发现任何问题时先修复并使用 `needs_more`；只有本轮未发现新问题、无需新增修改且验证仍通过时才能再次使用 `clean`。
{{end}}

写入：`{{.RepairPath}}`

写入后运行：`oz flow validate-repair --artifact "{{.RepairPath}}" --json`

严格 JSON：字段为 `summary`、`decision`、`evidence[]`、`findings[]`、`checks`、`non_blocking_findings[]`。
`decision` 只能是 `clean` 或 `needs_more`。发现并修正问题、仍需下一轮确认时用 `needs_more`；确认当前范围无已知问题时用 `clean`。
每个 finding 使用 `{title,severity,scope,evidence,recommendation}`。

同一角色续轮必须复用 backend-scoped 会话 `{{.RoleSessionKey}}`；repairer 不能自行归档，clean 后仍须独立 QA 放行。
