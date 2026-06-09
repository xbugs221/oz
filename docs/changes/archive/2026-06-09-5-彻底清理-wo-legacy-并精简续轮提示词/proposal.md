# 彻底清理 wo legacy 并精简续轮提示词

## 问题

`wo` 的公开执行路径已经收敛到内嵌 `go-dag`，但运行时代码、图导出、隐藏 node 子命令、规格和长期测试中仍保留 Dagu 相关残留。这些代码已经不再承担当前业务目标，却继续让维护者在阅读和修改 `wo` 工作流时误以为还有 Dagu executor、Dagu YAML 和节点子命令这套旧路径需要兼容。

同时，prompt 配置仍保留 `writing`、历史 `prompts/*.md` 快照回退和 `Legacy` 角色标记。这些内容主要是备份兼容，不再是当前工作流合同的一部分。继续保留会让配置合并、sealed run 恢复和角色表出现多套同义路径，也增加后续改 prompt 时的回归面。

最后，`review` 和 `fix` 角色的续轮提示词仍重复首轮已经给过的大段方法论、检查清单或格式说明。续轮应该依赖同一角色会话中的历史上下文，只提供当前轮次新增的 artifact、上一轮 findings、输出路径和必要格式边界，避免把首轮完整说明反复灌入上下文。

## 目标

- `wo` 运行时代码和当前规格只保留 `go-dag` 工作流，不再保留 Dagu executor、Dagu YAML exporter、Dagu CLI 调用或 Dagu node 入口。
- 当前长期测试和规格不再把 Dagu 当作需要验证的外部依赖或历史兼容对象。
- prompt 配置只支持当前角色键：`planning`、`execution`、`review`、`qa`、`fix`、`archive`。
- sealed run 恢复只读取 `prompt-snapshot.yaml` 中的当前角色键；缺失快照或缺少角色 prompt 时失败关闭，不再回退历史 `prompts/*.md` 或 `prompts.writing`。
- 移除只用于备份兼容的 legacy role/stage/prompt 分支。
- `wo-review.md` 和 `wo-fix.md` 首轮保留完整要求；第 2 轮及之后只保留增量输入和必要输出约束，不重复首轮方法论、示例和长清单。

## 非目标

- 不改变 `go-dag` 调度、review/QA/fix/archive 状态机和 artifact schema 的业务语义。
- 不清理 `docs/changes/archive/` 中作为历史审计材料存在的旧文字。
- 不要求删除 `go-dag` 这个当前 engine 名称。
- 不把 QA 提示词作为本次重点；只有在实现时发现共享去重机制必须同步调整时才做最小修改。
- 不为了兼容旧 run 而继续维护备份 prompt 快照或 Dagu 执行器。

## 验收

本提案通过以下真实测试和回归测试验收：

- `docs/changes/5-彻底清理-wo-legacy-并精简续轮提示词/tests/test_no_dagu_runtime_residue_contract.sh`
- `docs/changes/5-彻底清理-wo-legacy-并精简续轮提示词/tests/test_prompt_legacy_removed_contract.sh`
- `docs/changes/5-彻底清理-wo-legacy-并精简续轮提示词/tests/test_review_fix_resumed_prompt_compact_contract.sh`
- `go test ./...`

这些测试使用真实 `wo` 二进制、真实配置读取和真实 prompt 渲染路径。执行阶段不得通过删除测试、放宽断言或恢复旧兼容路径来通过验收。
