# Oz 流程证据闭环修复

## 问题与根因

历史运行把验收结果指向可变且被 Git 忽略的 `test-results/`。后续自查、测试和归档再次读取该目录时，覆盖、删除或特殊文件都会改变流程判断；同时合同没有要求可交付 demo，也没有约束代码、提案和证据必须属于同一提交。

## 修复

```text
执行 → 自查 × 2 → 测试 ⇄ 修复 → 归档
                 │
test-results → 运行态只写一次封存 → tests/evidence/proposals/<change>
```

- 验收轮次把日志、证据和结果封存到 `~/.local/state/oz/flow/.../evidence/`，后续阶段只读封存副本。
- `submission_evidence` 显式映射临时源与 Git 归档路径；每个提案至少包含一个 demo 视频。
- 归档从最终通过副本生成证据包，并要求代码、归档提案和证据位于 `delivery_base_head` 后唯一交付提交。
- 日常修复由 `fix-code`、`fix-webapp` 归集 `tests/evidence/fixes/<时间>-<主题>/before|after`。

## 对比与验证

- 修复前事实见 `before/state-summary.log`。
- 修复后结果见 `after/verification.log`。
- 当前旧提案必须先补充真实 demo 生成步骤及 `submission_evidence`，新版会在执行前阻断不完整合同，避免浪费长时间运行。
