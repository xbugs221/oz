## 规划任务
阶段：`{{.Stage}}`（第 `{{.Iteration}}` 轮）
运行目录：`{{.RunDirectory}}`
读取（相对此目录）：`state.json`、`acceptance.json`。

调用 `oz-plan` 技能开始讨论规划阶段。

规划必须把可演示能力与提交级证据包纳入验收设计：明确 demo 如何覆盖目标能力，并在 `submission_evidence` 中把可变 `source_path` 映射到 `tests/evidence/proposals/<change>/**` 下的 `archive_path`；`test-results/**` 只作为可变运行产物，不作为最终交付证据。
