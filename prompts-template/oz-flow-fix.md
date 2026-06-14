读取:

- `{{.StatePath}}`
- `{{.AcceptancePath}}`
- `{{.ReviewPath}}`
- 如存在，`{{.QAPath}}`

任务：

- 只修复当前 review/QA artifact 中列出的 findings。
{{if .IsFirstRoleTurn}}
- 必须做根因分析，禁止只按错误文本打补丁。
- 不得删除、弱化或绕过 `{{.AcceptancePath}}`。
{{end}}
{{if .HasRoleSession}}
- 复用当前角色会话：`{{.RoleSessionKey}}`
{{end}}
{{if .FixEscalated}}
- 升级轮次：连续失败 {{.ConsecutiveReviewFailures}} 次；summary 写上一轮未解决原因和重复 finding 根因。
{{end}}

写入：

```text
{{.FixSummaryPath}}
```

summary 用 Markdown，包含：修复的问题、根因、改动、验证命令及结果、剩余风险。
