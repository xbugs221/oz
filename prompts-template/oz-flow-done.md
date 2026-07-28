## 归档任务
阶段：`{{.Stage}}`（第 `{{.Iteration}}` 轮）
运行目录：`{{.RunDirectory}}`
读取（相对此目录）：`state.json`、`acceptance.json`、历史 audit/repair/QA artifacts。

执行：

- 调用 `oz-archive` skill 归档，change-name 见 `state.json.change_name`。

写入（相对运行目录）：`delivery-summary.md`。
summary 包含 `最终审核` 小节。
