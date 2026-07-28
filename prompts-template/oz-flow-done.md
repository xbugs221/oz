## 归档任务
阶段：`{{.Stage}}`（第 `{{.Iteration}}` 轮）
运行目录：`{{.RunDirectory}}`
读取（相对此目录）：`state.json`、`acceptance.json`、历史 audit/repair/QA artifacts。

执行：

- 调用 `oz-archive` skill 归档，change-name 见 `state.json.change_name`。
- 本阶段是最终 QA 后的只读边界，只允许机械移动提案；长期规格与规格测试必须已在最终 QA 前完成。
- 引擎会在进入归档前从最终通过的不可变快照自动提升证据；归档代理只核对最终提交级证据包位于 `tests/evidence/proposals/<change>/**`、没有命中 git ignore，并将其随本次归档提交，不得从 `test-results/**` 自行复制或重建。
- 必须从 state 封存的 `delivery_base_head` 新建且只新建一个完整交付 commit，使实现、归档提案与最终证据同属 HEAD；禁止 amend、squash 或沿用执行/自查阶段的提交。
- 归档命令返回后，严禁编辑、格式化或恢复提案目录及任何源码；可以原样暂存/提交命令结果，但不得为了“工作区干净”改写文件。
- 若归档后存在差异，保持原状并交给只读门禁判定。

写入（相对运行目录）：`delivery-summary.md`。
summary 包含 `最终审核` 小节。
