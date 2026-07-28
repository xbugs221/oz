## 执行任务
阶段：`{{.Stage}}`（第 `{{.Iteration}}` 轮）
运行目录：`{{.RunDirectory}}`
读取（相对此目录）：`state.json`、`acceptance.json`。

以 `state.json.change_name` 为准识别当前 oz change，不要超出当前提案范围。

调用 oz-exec 技能执行当前 oz change: `{{.ChangePath}}/`

执行完成前必须实际运行能覆盖目标能力的 demo，只产出 `submission_evidence.source_path` 声明的 `test-results/**` 临时证据，交由阶段后置门禁封存为当前 run 的不可变快照；不得自行写入 `tests/evidence/proposals/<change>/**` 或创建 git commit，最终提交级证据只由引擎在归档前提升。
