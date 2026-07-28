## 优化任务
阶段：`{{.Stage}}`（第 `{{.Iteration}}` 轮）
运行目录：`{{.RunDirectory}}`
读取（相对此目录）：`state.json`、`acceptance.json`；当前变更：`{{.ChangePath}}/`；diff baseline：`{{.BaselineHead}}`。

任务：

- 只处理当前提案范围：检查实现、修正发现的问题并运行确定性验证。
- `pre_qa_audit`（`audit_N`）：全量检查 acceptance、完整 diff、源码和测试；实际运行并确认 demo 覆盖目标能力，核对上一检查点的不可变证据快照（如有），只产出 `test-results/**` 临时源并交由本阶段后置门禁封存；不得写入 `tests/evidence/proposals/<change>/**` 提交级证据包。修复后继续自查，只有零新问题且 required tests 通过才可移交 QA。
- `qa_targeted_repair`（`targeted_repair_N`）：仅处理最新 QA findings、失败验收项及直接回归，不得扩审；对每个 finding 产出可复现的修复前失败证据与同一场景的修复后通过证据，按已封存 `submission_evidence` 映射更新临时源，交由本阶段后置门禁封存新快照。
- 每轮重新核对输入与验证；移交前复跑失败测试、全部 required tests 和 validation commands。失败或结果未绑定当前 diff 时不得 `clean`。
- 执行、自查和定向修复期间不得创建 git commit；完整交付提交只能由归档阶段创建。
- 当前提案问题写 `findings`；历史债务写 `non_blocking_findings`。
{{if .HasPreviousRepair}}
- 上一轮检查点：`{{.LatestPreviousRepairFile}}`。
{{end}}
{{if .HasPreviousQA}}
- 触发本轮的上一轮独立 QA：`{{.LatestPreviousQAFile}}`；必须逐项复核并处理其中的 findings。
{{end}}
{{if .IsRepairConfirmation}}
- 本轮是上一轮 `clean` 后的强制重审确认。必须重新从 state、acceptance、完整 diff 和验证结果开始审查，不得直接复述上一轮结论。
- 发现任何问题时先修复并使用 `needs_more`；只有本轮未发现新问题、无需新增修改且验证仍通过时才能再次使用 `clean`。
{{end}}

{{if not .RepairMode}}写入（相对运行目录）：`repair-{{.Iteration}}.json`

在运行目录中运行：`oz flow validate-repair --artifact "repair-{{.Iteration}}.json" --json`
{{end}}

严格 JSON：字段为 `summary`、`decision`、`evidence[]`、`findings[]`、`checks`、`non_blocking_findings[]`。
`decision` 只能是 `clean` 或 `needs_more`。发现并修正问题、仍需下一轮确认时用 `needs_more`；确认当前范围无已知问题时用 `clean`。
每个 finding 使用 `{title,severity,scope,evidence,recommendation}`。

同一角色续轮必须复用 backend-scoped 会话 `{{.RoleSessionKey}}`；repairer 不能自行归档，clean 后仍须独立 QA 放行。
