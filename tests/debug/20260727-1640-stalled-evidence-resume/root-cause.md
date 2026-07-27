# 停滞恢复绕过 evidence 语义进展

## 问题

`blocked_stalled` 恢复只重算原始 evidence 完整性哈希，任意时间、耗时等挥发
变化都能解除停滞。

## 根因

门禁阶段已分离原始 `EvidenceHash` 与语义 `EvidenceProgressHash`，恢复入口仍沿用
旧的单哈希判断。

## 修复

- 恢复时同时重算原始与语义 evidence 哈希。
- 原始哈希只更新完整性状态；只有既有语义哈希发生变化才算 evidence 进展。
- 同一失败下仅 Go 测试耗时变化保持阻塞，实质 evidence 内容变化才能恢复。

## 验证

- `go test ./internal/app -run '^TestRepairStalledBlockResumesWithEvidenceProgress$' -count=1`
