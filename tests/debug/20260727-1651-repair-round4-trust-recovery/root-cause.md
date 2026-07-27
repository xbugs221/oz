# 第四轮检查点与归档恢复信任缺口

## 问题

QA 检查点只绑定三个派生哈希，未覆盖 acceptance 的 coverage、diagnostics 与
validation 时间元数据；同代旧 `targeted_repair_N` 状态会在缺少新门禁时直接
报错。归档后置门禁失败后，恢复到新 audit 仍保留旧 archive invariant，导致
第二次归档永久阻塞。

## 根因

检查点信任使用结果摘要代替了持久 artifact 全内容；运行入口未迁移旧的定向
修复信任状态；fresh audit 只清理 QA 来源字段，没有撤销旧归档门禁。

## 修复

- QA 门禁绑定 validation 与 acceptance 两个持久 artifact 的精确字节。
- 运行旧或被篡改的定向修复前，fail-closed 路由到新 audit。
- fresh audit 删除旧 archive gate；归档成功持久化 `archive_read_only/passed`。
- 回归覆盖元数据篡改、旧门禁迁移和“归档失败→审计→QA→二次归档”。

## 验证

- `go test ./internal/app -count=1`
- `bash docs/changes/45-收敛全量自查与QA定向修复闭环/tests/test_quality_delivery_loop.sh`
