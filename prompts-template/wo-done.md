读取当前 run 目录：

- `{{.StatePath}}`
- `{{.AcceptancePath}}`
- `{{.PreviousReviewPaths}}`
- `{{.PreviousQAPaths}}`（如存在）
- `{{.PreviousFixSummaryPaths}}`（如存在）

从 `state.json.change_name` 获取 `<change-name>`。必须调用 `oz-archive` skill，并以该 skill 的说明作为归档流程依据。

归档前必须确认：

- 最新 review 为 clean，且 evidence 引用了 validation artifact 和运行时/截图/trace/QA 证据
- 最新 QA 为 clean，且 `acceptance_matrix` 逐项覆盖 `{{.AcceptancePath}}` 的 `required_tests` / `required_evidence`
- 最新 QA evidence 包含可复核的测试、截图、trace、控制台或网络检查结果
- 如配置了 validation.commands，execution/fix 阶段均有 passed 记录

完成后写入：

```text
{{.DeliverySummaryPath}}
```

该路径是当前 run 的 `delivery-summary.md`。

summary 必须包含 `最终审核` 小节，便于人工快速复核归档质量；至少写清：

- 提案目的
- 预期效果
- 边界
- 审核证据
- 快速实测
- 通过标准

**重要约束**

- 代码实现通过是否完成 archive 只检查两件事：
  - `{{.DeliverySummaryPath}}` 存在
  - `docs/changes/archive/*-<change-name>` 存在
- 不要覆盖已有归档目录
- git commit 只包含本次 oz 变更相关内容
