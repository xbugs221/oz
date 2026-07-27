# 后台 worker 交接失败误触发状态回滚

## 问题

恢复入口在后台启动函数返回错误时回滚原阻塞状态，但进程句柄释放失败发生在
worker 已成功启动之后；此时回滚会与运行中的 worker 争用同一份状态。

## 根因

后台启动函数用普通 error 同时表示 `cmd.Start` 前失败和 `Process.Release`
失败，调用方无法判断 worker 是否已经取得执行资格。

## 修复

- 为后台启动错误记录 `ProcessStarted` 状态。
- 仅在进程尚未启动时回滚 run/batch；已启动但交接失败时保留 prepared 状态。
- 覆盖真实进程释放失败包装，以及 resume、restart、batch 三个恢复入口。

## 验证

- `go test ./internal/app -count=1`
- `bash docs/changes/45-收敛全量自查与QA定向修复闭环/tests/test_quality_delivery_loop.sh`
