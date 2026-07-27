# 第六轮 QA 定向修复：恢复原子性与观测边界

## 问题

后台恢复在进程启动失败后留下已清除阻塞的运行态；未跟踪目录折叠、
环境 marker 前缀、动态图排序和阻塞状态映射还存在边界缺口。

## 根因

- 恢复准备与后台进程启动之间没有失败回滚。
- Git 状态快照沿用默认的目录级未跟踪摘要。
- 脱敏只替换 marker 后缀，保留了不可信前缀。
- 动态图使用阶段种类排序，状态视图只识别 audit/targeted repair。

## 修复

- 后台 resume、restart 与 batch restart 启动失败时恢复原 run/batch 状态。
- Git 快照逐文件记录未跟踪路径，拒绝既存目录中的未声明新增文件。
- marker 行只持久化固定 marker 与安全环境名。
- 动态图优先按持久化开始时间排序；阻塞映射覆盖 audit、定向修复、QA、归档。
- 为五项 QA finding 增加确定性回归。

## 验证

- `go test ./internal/app -count=1`
- `bash docs/changes/45-收敛全量自查与QA定向修复闭环/tests/test_quality_delivery_loop.sh`
