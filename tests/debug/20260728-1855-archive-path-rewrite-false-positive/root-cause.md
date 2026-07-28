<!-- 文件目的：记录 oz archive 确定性路径重写被只读门禁误判的根因与修复。 -->

# 归档路径重写被误判为内容修改

## 场景与证据

最终 QA 已通过后，归档阶段仅执行 `oz archive <change> --yes`。命令会移动提案目录，并把提案正文中的 active 路径和相对 `tests/` 引用改写到带日期的 archive 路径。归档只读门禁随后报告“archive 修改了最终 QA 后的源码或提案内容”。

## 根因

置信度：Confirmed。

归档 invariant 只 canonical 化了提案目录名，仍直接比较文件原始内容哈希。因此 CLI 的确定性路径改写会改变文件哈希，即使业务语义、源码和 evidence 都没有变化。

## 修复

- 单独遍历当前提案，使用 `@current-change/` canonical 路径生成内容快照。
- 同时 canonical 化 active/archive 绝对引用与 CLI 生成的 proposal-local `tests/` 引用。
- 恢复归档提案时先按封存哈希验证路径逆变换，再把引用还原为 active 路径。
- 兼容旧版归档曾误改写的根目录 `tests/spec/` 引用：仅当归档提案内不存在目标文件时还原。
- 不忽略其他文本变化；归档阶段修改提案语义、源码或 required evidence 仍进入安全暂停。

## 回归测试

- 目录移动并执行确定性引用改写：归档门禁通过。
- 归档阶段修改根源码：门禁拒绝。
- 归档阶段追加提案语义：门禁拒绝。
- 机械改写后的 acceptance 可通过封存校验并恢复为原始内容。
- required evidence 或最终 QA checkpoint 变化：既有回归继续拒绝。

## 剩余风险

若未来 `oz archive` 新增另一类确定性文档迁移，必须同步扩展 canonical 规则和真实命令级回归，不能用放宽整个归档目录来规避。
