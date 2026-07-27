# Detached 恢复事务与 dirty 内容检查点缺口

## 场景与证据

- archive 阻塞恢复先移动提案，后续 preparation 失败时会留下 active 提案。
- detached 启动失败使用整份旧 state 回滚，可覆盖另一执行者已经保存的阶段进展。
- `git status --porcelain` 对既存 dirty 文件内容变化保持同一状态行，环境恢复会漏检。

## 根因与置信度

根因是 worker 启动前补偿只覆盖启动函数错误，且没有绑定准备后的持久状态代次；
环境恢复检查点只保存路径状态，没有保存 dirty 路径内容。隔离反例稳定复现，
置信度高。

## 修复

- preparation 任一步失败都进入同一补偿路径，恢复 run、batch 和提案位置。
- 回滚先取得 run lease，再以准备后 state 文件哈希执行 CAS；代次变化时拒绝旧回写。
- 环境阻塞时保存 dirty 路径内容哈希，恢复时与 porcelain 路径差异合并校验。

## 回归与验证

- `TestArchiveBlockedDetachedPreparationFailureRestoresProposalLocation`
- `TestRecoveryDetachedRollbackRejectsConcurrentStateProgress`
- `TestEnvironmentResumeRejectsPreexistingDirtyContentChanges`
- `go test ./internal/app -count=1`
- `bash docs/changes/45-收敛全量自查与QA定向修复闭环/tests/test_quality_delivery_loop.sh`

## 剩余风险

跨 run、batch 和提案目录的补偿仍依赖幂等写入，不提供跨文件系统原子提交；CAS
与 lease 保证不会用陈旧快照覆盖已开始的 worker，后续异常可从原阻塞状态重试。
