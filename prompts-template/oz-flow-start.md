## 执行任务
阶段：`{{.Stage}}`（第 `{{.Iteration}}` 轮）
运行目录：`{{.RunDirectory}}`
读取（相对此目录）：`state.json`、`acceptance.json`。

以 `state.json.change_name` 为准识别当前 oz change，不要超出当前提案范围。

调用 oz-exec 技能执行当前 oz change: `{{.ChangePath}}/`
