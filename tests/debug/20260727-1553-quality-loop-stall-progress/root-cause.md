# 质量循环停滞进展与恢复门禁修复

## 场景

变更 45 的全量自查发现：重复失败的测试日志包含耗时、时间戳或尝试序号时，
原始内容哈希会持续变化；QA/archive 因证据变化解除阻塞后又会继续消费旧门禁。

## 证据

- `TestsHash`、`ValidationHash` 和 `EvidenceHash` 同时承担防篡改与进展判断，
  `go test` 的 `0.003s/0.004s` 差异可被误判为修复进展。
- QA/archive 恢复只在源码 diff 变化时重新路由；证据变化仍恢复原阶段，
  随后旧 acceptance checkpoint 与新 evidence hash 必然冲突。
- QA 阻断后直接进入定向修复时沿用上一轮 finding 指纹，prompt 会拒绝新 QA。

## 根因与置信度

根因是“原始证据完整性”与“语义进展”复用了同一哈希，同时恢复路由只识别源码
变化。置信度：高。

## 修复方案

- 保留原始日志、验证和证据哈希用于持久门禁完整性校验。
- 新增语义进展哈希，仅规范化测试耗时、运行时间戳和尝试序号，保留错误正文变化。
- 证据变化或人工重启后的 QA/archive 先重新进入 audit/定向修复门禁。
- 定向恢复只接受覆盖封存 acceptance 的有效 QA，并同步 finding 指纹。

## 回归测试

- `go test ./internal/app -count=1`
- `bash docs/changes/45-收敛全量自查与QA定向修复闭环/tests/test_quality_delivery_loop.sh`
- `go test ./... -count=1`

## 剩余风险

稳定化规则只处理已确认的运行噪声；未来新增工具若产生其它挥发格式，应增加对应
规范化回归，不能直接放宽原始完整性校验。
