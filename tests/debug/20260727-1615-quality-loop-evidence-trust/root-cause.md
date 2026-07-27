# 质量循环证据进展与来源 QA 信任修复

## 场景

续轮审查发现：带时区偏移的时间戳仍能制造伪进展；required evidence 的实质内容
变化又不会计入语义进展；定向修复只用简化 finding 指纹识别来源 QA。

## 证据

- RFC3339 规范化只匹配 `Z`，遗漏 `+08:00`、`-05:00`。
- `EvidenceProgressHash` 只包含 evidence 的 ID、路径和存在状态，A→B 内容变化会在
  本轮停滞后丢失，恢复时只能观察到 B→B。
- `qaFindingFingerprint` 不包含 finding evidence/recommendation 与 acceptance matrix
  证据；未通过 QA 只读门禁的 artifact 也可在恢复分支进入定向修复。

## 根因与置信度

根因是完整性、稳定停滞语义和来源 artifact 信任仍有部分职责复用或缺失。
置信度：高。

## 修复方案

- 规范化带 `Z` 或数值时区的 RFC3339 时间戳及 JSON `duration_ms`。
- 为文本 evidence 计算稳定内容哈希；二进制 evidence 保留原始内容哈希。
- 单独持久化来源 QA 完整内容哈希；成功 QA 只读门禁记录 `passed` 和输入 diff。
- 恢复时仅允许完整 QA 哈希、只读门禁和 durable checkpoint 同时可信的 artifact
  进入定向修复，否则重开全量 audit。

## 回归测试

- `TestQualityFailureFingerprintsIgnoreVolatileLogs`
- `TestQualityLoopEvidenceContentChangeCountsAsProgress`
- `TestQABlockedResumeWithSourceProgressRoutesToTargetedRepair`
- `TestQABlockedResumeWithUntrustedArtifactRoutesToFreshAudit`
- `go test ./internal/app -count=1`

## 剩余风险

未来新增 evidence 格式或运行噪声字段时必须新增确定性样例；未知二进制内容变化仍
按实质进展处理，不能为了停滞收敛而弱化原始证据完整性。
