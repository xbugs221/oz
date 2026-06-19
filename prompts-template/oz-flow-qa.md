读取：`{{.StatePath}}`、`{{.AcceptancePath}}`、`{{.ReviewPath}}`、当前变更 `{{.ChangePath}}/`

任务：

- 只验收当前提案范围，不修改源码或 `{{.AcceptancePath}}`。
- `acceptance_matrix[].id` 必须逐字来自 `{{.AcceptancePath}}` 的 required_tests/required_evidence，并覆盖 acceptance_contract。
- 当前提案问题写 `findings`；历史债务或无关问题写 `non_blocking_findings`，scope 用 `out_of_scope_existing`。
- blocking scope 只允许 `current_change` 或 `introduced_regression`；required_evidence 只要求可复核，不要求运行产物进 git。

写入：`{{.QAPath}}`

写入后运行：`oz flow validate-qa --artifact "{{.QAPath}}" --acceptance "{{.AcceptancePath}}" --json`

{{if .IsFirstRoleTurn}}
只写一个 JSON object。

字段：`summary`、`decision`(0=clean,1=needs_fix)、`evidence[]`、`acceptance_matrix[]`、`findings[]`、`non_blocking_findings[]`。

`acceptance_matrix[]`: `{id,status,artifact,evidence}`，`status`: 0=passed, 1=failed。`findings[]`: `{title,severity,scope,evidence,recommendation}`；`severity`: 1=blocker, 2=major, 3=minor；`scope`: 1=current_change, 2=introduced_regression, 0=out_of_scope_existing。`evidence[]` 每项必须是字符串，写可复核测试、截图、trace、控制台、网络或运行时证据。

clean：`decision=0`、`findings=[]`、`evidence` 非空、`acceptance_matrix` 覆盖 required_tests/required_evidence。needs_fix：`decision=1` 且 `findings` 非空。
{{else}}
续轮：复用当前角色会话 `{{.RoleSessionKey}}`，按同 schema 重写 `{{.QAPath}}`。
{{end}}
