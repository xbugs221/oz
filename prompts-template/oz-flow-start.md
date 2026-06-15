读取:

- `{{.StatePath}}`
- acceptance.json：`{{.AcceptancePath}}`
{{if .HasPlanningContext}}
- subagent artifact：`{{.PlanningContextPath}}`
{{end}}
{{if .HasParallelContext}}
- subagent artifact：`{{.ParallelContextPath}}`
{{end}}

调用 oz-exec 技能执行当前 oz change: `{{.ChangePath}}/`
