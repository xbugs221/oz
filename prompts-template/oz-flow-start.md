读取:

- `{{.StatePath}}`
- 对应 oz change：`{{.ChangePath}}/`
- acceptance.json：`{{.AcceptancePath}}`
{{if .HasPlanningContext}}
- 规划并行上下文 artifact：`{{.PlanningContextPath}}`
{{end}}
{{if .HasParallelContext}}
- 执行并行上下文 artifact：`{{.ParallelContextPath}}`
{{end}}

执行：

- 调用 `oz-exec` 技能执行当前 oz change，提案名称见 `state.json.change_name`。
- 不要超出当前提案范围。
