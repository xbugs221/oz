# 32-拆分工作流Engine运行边界

当前 `internal/app/state.go` 同时定义持久化状态模型、`Engine`、run/resume 入口、运行循环、阶段执行、进度输出和 session 持久化。它是工作流最核心路径，文件过大导致后续改运行态恢复、stage 执行或 progress writer 时需要在同一文件中穿梭。

本次交付目标是按运行职责拆分 `Engine` 相关代码，保持当前 workflow 行为、状态 JSON 字段和已有测试合同不变。非目标是不改状态机决策、不改 Go DAG 调度、不改外部 CLI 命令。

执行阶段默认先运行 `bash docs/changes/archive/2026-06-14-32-拆分工作流Engine运行边界/tests/engine_boundary_test.sh`，确认当前实现失败于结构边界缺失，再完成拆分并让测试通过。验收证据写入 `test-results/32-engine-boundary/contract.log`。
