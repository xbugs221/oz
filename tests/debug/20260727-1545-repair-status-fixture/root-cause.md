# repair-v1 状态视图回归测试夹具修复

## 场景

变更 45 的全量验收中，`go test ./internal/app -count=1` 在
`TestVisibleSessionItemsDeduplicatesSharedRepairer/repair-v1` 失败：修复者状态行数量为 0，预期为 1。

## 证据

- 失败日志：`repairer rows = 0, want 1`。
- `observedStatusStages` 对有限 `repair-v1` 只读取封存工作流阶段，不动态合并 `state.Stages`。
- 测试把 generation 改为 `repair-v1`，但保留了 `MaxRepairIterations = 0`，因此封存配置不包含 `repair_1`。

## 根因与置信度

根因是测试夹具构造了不一致的旧代快照，而非状态视图去重逻辑错误。置信度：高。

## 修复方案

在 `repair-v1` 子用例中声明一轮有限修复，使 `repair_1` 属于封存阶段；`quality-loop-v1`
子用例继续通过动态阶段发现覆盖新工作流。

## 回归测试

- `go test ./internal/app -run '^TestVisibleSessionItemsDeduplicatesSharedRepairer$' -count=1`
- `go test ./internal/app -count=1`
- `bash docs/changes/45-收敛全量自查与QA定向修复闭环/tests/test_quality_delivery_loop.sh`

## 剩余风险

无已知功能风险；旧代状态仍严格依赖其封存的有限阶段配置。
