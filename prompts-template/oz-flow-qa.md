## QA 任务
阶段：`{{.Stage}}`（第 `{{.Iteration}}` 轮）
运行目录：`{{.RunDirectory}}`
读取（相对此目录）：`state.json`、`acceptance.json`、{{if .HasRepairCheckpoint}}最新 repair 检查点，{{else}}本轮未配置 repair 检查点，{{end}}当前变更：`{{.ChangePath}}/`；diff baseline：`{{.BaselineHead}}`。

任务：

- 只验收当前提案范围，不修改源码或封存 `acceptance.json`。
- 使用独立 QA 会话核对{{if .HasRepairCheckpoint}}最新 repair 检查点{{else}}执行结果（零轮 repair 模式无 repair 检查点）{{end}}；不得继承 repairer 的自我判断。
- QA 打回时必须在 findings 和 acceptance_matrix 中给出可复现的失败证据；下一阶段只会据此进行定向修复。
- 必须实际复核 demo 覆盖目标能力及当前 run 内封存的 required_evidence 不可变快照；保持只读，不得修改 `test-results/**` 临时源或写入 `tests/evidence/proposals/<change>/**`。{{if .HasRepairCheckpoint}}若本轮验收修复结果，逐项确认每个 finding 同时具有可复现的修复前失败证据与同一场景的修复后通过证据。{{end}}
- `acceptance_matrix[].id` 必须逐字来自封存 `acceptance.json` 的 required_tests/required_evidence，并覆盖 acceptance_contract。
- 当前提案问题写 `findings`；历史债务或无关问题写 `non_blocking_findings`，scope 用 `out_of_scope_existing`。
- blocking scope 只允许 `current_change` 或 `introduced_regression`；不得读取可变 `source_path` 判定漂移，也不能把 `test-results/**` 哈希变化当作源码回退信号。

写入（相对运行目录）：`qa-{{.Iteration}}.json`

在运行目录中运行：`oz flow validate-qa --artifact "qa-{{.Iteration}}.json" --acceptance "acceptance.json" --json`

{{if .IsFirstRoleTurn}}
只写一个 JSON object。

字段：`summary`、`decision`(0=clean,1=needs_fix)、`evidence[]`、`acceptance_matrix[]`、`findings[]`、`non_blocking_findings[]`。

`acceptance_matrix[]`: `{id,status,artifact,evidence}`，`status`: 0=passed, 1=failed。`findings[]`: `{title,severity,scope,evidence,recommendation}`；`severity`: 1=blocker, 2=major, 3=minor；`scope`: 1=current_change, 2=introduced_regression, 0=out_of_scope_existing。`evidence[]` 每项必须是字符串，写可复核测试、截图、trace、控制台、网络或运行时证据。

clean：`decision=0`、`findings=[]`、`evidence` 非空、`acceptance_matrix` 覆盖 required_tests/required_evidence。needs_fix：`decision=1` 且 `findings` 非空。
{{else}}
续轮：复用当前角色会话 `{{.RoleSessionKey}}`，按同 schema 重写 `qa-{{.Iteration}}.json`。
{{end}}
