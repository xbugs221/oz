读取 `{{.StatePath}}`，并调用 `oz-exec` 技能开始执行当前 oz change，提案名称见 `state.json.change_name` 字段。

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
