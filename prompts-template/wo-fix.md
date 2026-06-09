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
- 首轮修复必须逐项建立“finding -> 根因 -> 改动 -> 验证”的对应关系；如果一个 finding 涉及多个模块，先确认边界和数据流，再选择最小实现点。
- 修改前先判断失败来自需求理解、状态机、配置合并、文件路径、artifact schema、外部命令调用还是测试过期；不要把症状当成根因。
- 修改后必须回读相关 proposal/design/spec/task 和 acceptance，确认实现没有偏离 sealed run 的验收合同，也没有扩大到无关重构。
- 若发现测试与新意图冲突，先更新测试意图记录，再实现。
- 改正后，必须运行修复相关的测试先行验证
- 不得删除、弱化或绕过 `{{.AcceptancePath}}`；如果 finding 指出验收合同本身缺项，应在 summary 中说明需要回到 oz-create 更新提案，而不是在 sealed run 内临时改合同
- 普通修复轮次不需要读取所有旧 review/fix artifact
{{else}}
- 复用当前角色会话：`{{.RoleSessionKey}}`
- 读取当前 review：`{{.ReviewPath}}`
{{if .QAPath}}
- 如存在，读取当前 QA：`{{.QAPath}}`
{{end}}
- 只修复当前 review/QA artifact 中列出的 findings
- 运行与本轮修复相关的验证，并把结果写入 `{{.FixSummaryPath}}`
{{end}}

{{if .FixEscalated}}
升级轮次：连续失败 {{.ConsecutiveReviewFailures}} 次，本轮 reasoning 为 {{.FixEscalationReasoning}}；summary 中说明上一轮未解决原因并核对重复 finding 根因。
{{end}}

完成后写入：

```text
{{.FixSummaryPath}}
```

summary 内容使用 Markdown，包含修复的问题、验证命令及结果；有剩余风险时写明。
