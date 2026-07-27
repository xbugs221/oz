# 质量门禁检查点进展哈希兼容修复

## 场景

上一轮为 `quality-loop-v1` acceptance 测试结果新增 `log_progress_hash`，用于把
挥发日志从停滞进展判断中分离。已经持久化的同代检查点没有该派生字段。

## 证据

`validateQualityAcceptanceCheckpointTests` 在校验可信原始日志哈希后仍强制
`log_progress_hash` 非空。升级前已通过的检查点因此会在 QA/archive 重放时被误判
为“进展日志哈希不一致”，并进入 `blocked_stalled`。

## 根因与置信度

根因是同一 workflow generation 新增可确定性派生字段时没有保留缺字段读取兼容。
原始 `log_hash` 与日志内容已提供完整性保证，进展哈希可由可信日志重算。置信度：高。

## 修复方案

- 继续强制校验原始日志内容与 `log_hash`。
- 旧检查点缺少 `log_progress_hash` 时，从已验证日志确定性派生。
- 新检查点存在该字段时，仍要求它与派生值完全一致。

## 回归测试

- `TestVerifyQualityAcceptanceCheckpointAcceptsLegacyProgressHash`
- `TestVerifyQualityAcceptanceCheckpointRejectsTampering/test_progress_hash`
- `bash docs/changes/45-收敛全量自查与QA定向修复闭环/tests/test_quality_delivery_loop.sh`
- `go test ./internal/app -count=1`

## 剩余风险

未来若新增其它可派生持久字段，应同时定义同代旧检查点的读取策略；不可降低原始
日志、结果或 sealed acceptance 的完整性校验。
