读取:

- `{{.StatePath}}`
- 对应 oz change：`{{.ChangePath}}/`
- 验收合同：`{{.AcceptancePath}}`

执行：

- 调用 `oz-exec` 技能开始执行当前 oz change，提案名称见 `state.json.change_name` 字段。
- 先确认 `{{.ChangePath}}/` 已提交到 git；如果提案目录尚未提交，先只提交该提案目录，避免执行阶段误删或改弱合同。
- 默认读取 `brief.md`、`acceptance.json` 和 `tests/` 中创建阶段写好的契约测试；`proposal.md`、`design.md`、`spec.md`、`task.md` 只在验收合同冲突、用户最新意图冲突、历史测试需要更新或实现路径存在架构分歧时按需读取。
- 以当前提案和用户最新意图为准；如果历史测试与新意图冲突，更新历史测试并在提案文档或任务中记录原因。
- 先运行 `{{.AcceptancePath}}` 中 `required_tests[].command` 指向的创建阶段契约测试；功能尚未实现时，失败原因应指向目标行为缺失，而不是测试语法、路径或环境配置错误。
- 不得删除、弱化、跳过或改写创建阶段的契约测试和 `{{.AcceptancePath}}` 来让实现过关。
- 可以新增补充测试，但必须是真实项目测试代码；新增契约测试写入当前 change 的 `tests/`，端到端或回归测试按项目惯例写入根目录测试集，并同步更新 `{{.AcceptancePath}}`。
- 按 `task.md` 完成实现后更新复选框；execution 阶段完成标准是 `oz status {{.ChangeName}} --json` 中 `tasks.total > 0` 且 `tasks.done == tasks.total`。
- 结束前运行相关测试，保留可复核命令、输出和产物路径，供后续 review/QA 使用。

边界：

- 只实现当前 oz change；不要顺手重构无关模块。
- 不修改 `docs/changes/archive/` 中的历史归档，除非当前提案明确要求。
- 不伪造测试、截图、trace、runtime log 或 acceptance matrix。
- 不把当前阶段失败转移给 review/QA/fix；执行阶段应把实现、测试和 task 状态交付到可审核状态。

{{if .HasPlanningContext}}
规划并行上下文 artifact：

- 如已存在 `{{.PlanningContextPath}}`，先读取其中的成员摘要、证据路径和失败状态，再开始实现。
- 如尚不存在该 artifact，按 `workflow_config.parallel.groups.planning_context` 的成员职责并行收集只读上下文，并写入 `{{.PlanningContextPath}}`。
- tool/subagent 只作为提示词角色线索，不作为 CLI 参数；当前主 agent 负责按成员 `name/purpose/stage/tool/subagent` 组织产物。
- advisory 成员失败时记录失败摘要并继续主执行；required 成员失败时在实现交付中明确标为 blocked 风险，不得伪造成功。
{{end}}

{{if .HasParallelContext}}
并行上下文增强已启用：

- 如已存在 `{{.ParallelContextPath}}`，先读取其中的成员摘要、证据路径和失败状态，再开始实现。
- 如尚不存在该 artifact，先按 `workflow_config.parallel.groups.implementation_context` 的成员职责并行收集只读上下文，并写入 `{{.ParallelContextPath}}`。
- tool/subagent 只作为提示词角色线索，不作为 CLI 参数；当前主 agent 负责按成员 `name/purpose/stage/tool/subagent` 组织产物。
- advisory 成员失败时记录失败摘要并继续主执行；required 成员失败时在实现交付中明确标为 blocked 风险，不得伪造成功。
{{end}}
