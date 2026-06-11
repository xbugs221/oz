读取:

- `{{.StatePath}}`
- `{{.AcceptancePath}}`
- `{{.ReviewPath}}`
- 当前变更：`{{.ChangePath}}/`
{{if .HasPlanningContext}}
- `{{.PlanningContextPath}}`
{{end}}
{{if .HasParallelContext}}
- `{{.ParallelContextPath}}`
{{end}}
{{if .HasParallelQA}}
- `{{.ParallelQAPath}}`
{{end}}

任务：

- 只验收当前提案范围，不修改源码或 `{{.AcceptancePath}}`。
- 当前提案问题写 `findings`；历史债务或无关问题写 `non_blocking_findings`。
- `acceptance_matrix[].id` 必须逐字来自 `{{.AcceptancePath}}`。
- `required_evidence` 只要求可复核，不要求进入 git。不要把 `test-results/`、截图、trace 或 runtime log 当作应提交产物；如果合同强制这些运行产物被 git 跟踪，QA 应判定合同错误。
{{if .HasParallelQA}}
- 读或生成 `{{.ParallelQAPath}}`；缺少 required evidence 或有 severity=1/2 finding 时，最终 `decision` 不得为 0。
{{end}}

写入：

```text
{{.QAPath}}
```

{{if .IsFirstRoleTurn}}
只写一个 JSON object，字段规则：

- `summary`
- `decision`: 0=clean, 1=needs_fix
- `evidence[]`: 可复核测试、截图、trace、控制台、网络或运行时证据
- `acceptance_matrix[]`: `{id,status,artifact,evidence}`，`status`: 0=passed, 1=failed
- `findings[]`: `{title,severity,scope,evidence,recommendation}`
- `severity`: 1=blocker, 2=major, 3=minor
- `scope`: 1=current_change, 2=introduced_regression, 0=out_of_scope_existing
- `non_blocking_findings[]`: 仅 scope=0

clean 要求：`decision=0`、`findings=[]`、`evidence` 非空、`acceptance_matrix` 覆盖 acceptance_contract 的 required_tests/required_evidence。
needs_fix 要求：`decision=1`、`findings` 至少一项。
{{else}}
续轮：

- 复用当前角色会话：`{{.RoleSessionKey}}`
- 按首轮同 schema 重写 `{{.QAPath}}`。
{{end}}
