读取:

- `{{.StatePath}}`
- `{{.AcceptancePath}}`
- `{{.ReviewPath}}`
- 如存在，`{{.QAPath}}`

任务：

- 只修复当前 review/QA artifact 中列出的 findings
{{if .IsFirstRoleTurn}}
- 充分理解评审意见，明晰意图
- 根据评审意见列出的问题根因逐项验证并修复
- 从根源入手，不能治标不治本
- 必须做根因分析，禁止只按错误文本打补丁
{{else}}
- 复用当前角色会话：`{{.RoleSessionKey}}`
- 根据当前 findings 做最小必要修复
{{end}}
- 改正后，必须运行修复相关的测试先行验证
- 不得删除、弱化或绕过 `{{.AcceptancePath}}`；如果 finding 指出验收合同本身缺项，应在 summary 中说明需要回到 oz-create 更新提案，而不是在 sealed run 内临时改合同
- 普通修复轮次不需要读取所有旧 review/fix artifact

{{if .FixEscalated}}
自动升级：

- 连续 review 失败次数：{{.ConsecutiveReviewFailures}}
- 本轮 reasoning：{{.FixEscalationReasoning}}
- 需要说明上一轮为什么没解决，并验证重复 finding 是否来自同一根因
{{if .RepeatedFindingTitles}}
- 重复 findings：
{{range .RepeatedFindingTitles}}
  - {{.}}
{{end}}
{{end}}
{{end}}

完成后写入：

```text
{{.FixSummaryPath}}
```

summary 内容使用 Markdown，包含：

- 修复的问题
- 运行的验证命令及结果
- 剩余风险；没有则跳过不写
