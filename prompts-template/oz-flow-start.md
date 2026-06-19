读取:

- `{{.StatePath}}`
- 以 `state.json.change_name` 为准识别当前 oz change，不要超出当前提案范围
- acceptance.json：`{{.AcceptancePath}}`

调用 oz-exec 技能执行当前 oz change: `{{.ChangePath}}/`
