## 修复任务
阶段：`{{.Stage}}`（第 `{{.Iteration}}` 轮）
运行目录：`{{.RunDirectory}}`
读取（相对此目录）：`state.json`、`acceptance.json`、`review-{{.Iteration}}.json`{{if .QAPath}}、`qa-{{.Iteration}}.json`{{end}}。

任务：

- 只修复当前 review/QA artifact 中列出的 findings。
{{if .IsFirstRoleTurn}}- 必须做根因分析，禁止只按错误文本打补丁；不得删除、弱化或绕过封存 `acceptance.json`。
{{end}}{{if .HasRoleSession}}- 复用当前角色会话：`{{.RoleSessionKey}}`
{{end}}{{if .FixEscalated}}- 升级轮次：连续失败 {{.ConsecutiveReviewFailures}} 次；summary 写上一轮未解决原因和重复 finding 根因。
{{end}}

写入（相对运行目录）：`fix-{{.Iteration}}-summary.md`

summary 用 Markdown，包含：修复的问题、根因、改动、验证命令及结果、剩余风险。
