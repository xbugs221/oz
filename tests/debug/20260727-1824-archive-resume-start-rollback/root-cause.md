# Archive 恢复启动失败未回滚提案位置

## 问题

archive 阶段阻塞后恢复到 fresh audit 时会先把已归档提案移回 active；
后台 worker 启动失败只恢复 run/batch 状态，未恢复提案目录位置。

## 根因

detached 回滚快照只包含 `state.json`，没有记录恢复准备阶段执行的提案目录重命名。

## 修复

- detached run 回滚同时记录原 active/archived 提案位置。
- 启动前失败时先恢复提案目录，再恢复 run/batch 状态。
- 已启动 worker 的交接错误继续保留 prepared 状态，不执行位置回滚。
- 覆盖 resume、restart、batch 三个 archive 阻塞恢复入口。

## 验证

- `go test ./internal/app -count=1`
- `bash docs/changes/45-收敛全量自查与QA定向修复闭环/tests/test_quality_delivery_loop.sh`
