# 第五轮 QA 输入冻结与旧制品迁移缺口

## 问题

QA 运行前只冻结 diff，检查点内容可在 QA 执行期间变化后被新的哈希接受；
同时旧定向修复若已有合法 stage artifact，会在迁移检查前被当作完成而跳过。

## 根因

QA 检查点哈希首次计算发生在 runner 返回后；主循环先判断 artifact 已完成，
再执行来源 QA 信任迁移。

## 修复

- QA 启动前同时冻结 diff 与完整检查点，并在 runner 前后复核相同哈希。
- 主循环在 artifact 快捷完成判断前执行旧信任迁移。
- 迁移时清理旧定向修复的 stage、计时、DAG 与关联 gate，再转入新 audit。
- 回归覆盖 QA 期间检查点漂移，以及携带合法旧 artifact 的恢复路径。

## 验证

- `go test ./internal/app -count=1`
- `bash docs/changes/45-收敛全量自查与QA定向修复闭环/tests/test_quality_delivery_loop.sh`
