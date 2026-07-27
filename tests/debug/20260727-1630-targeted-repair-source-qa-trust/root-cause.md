# 定向修复来源 QA 信任链缺口

## 问题

`targeted_repair_N` 的提示词会校验来源 QA 内容哈希，却不会在消费点重放 QA
只读门禁及其 validation/acceptance 持久检查点。正常完成链路还会把已通过的
`qa_read_only` 门禁覆盖为普通 `artifact` 门禁。最终 clean QA 进入 archive
时也未封存其完整内容。

## 根因

恢复分支已集中使用 `qualityLoopTrustedSourceQA`，提示词与 archive 分支仍保留
较早的局部判断；同时普通 artifact 清理与 QA 只读门禁复用了同一状态槽，
导致生产完成态丢失信任类型。

## 修复

- 提示词生成复用统一的来源 QA 信任校验。
- 保留已通过的 QA/归档只读门禁，不再被普通 artifact 清理覆盖。
- clean QA 同样封存完整内容，archive 前后均重放 QA、acceptance、只读门禁和
  持久检查点信任。
- 增加回归：篡改 QA 对应的持久 acceptance 检查点后，定向修复提示词必须
  fail-closed；恢复检查点后才能继续。

## 验证

- `go test ./internal/app -run '^TestQABlockedResumeWithSourceProgressRoutesToTargetedRepair$' -count=1`
- `bash docs/changes/45-收敛全量自查与QA定向修复闭环/tests/test_quality_delivery_loop.sh`
