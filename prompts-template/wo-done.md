读取：

- `{{.StatePath}}`
- `{{.AcceptancePath}}`
- `{{.PreviousReviewPaths}}`
- `{{.PreviousQAPaths}}`（如存在）
- `{{.PreviousFixSummaryPaths}}`（如存在）

执行：

- 调用 `oz-archive` skill 归档，change-name 见 `state.json.change_name`。

写入：

```text
{{.DeliverySummaryPath}}
```

该路径是当前 run 的 `delivery-summary.md`。
summary 包含 `最终审核` 小节。
