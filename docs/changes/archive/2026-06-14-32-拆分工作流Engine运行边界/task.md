# 任务

- [x] 先运行 `bash docs/changes/archive/2026-06-14-32-拆分工作流Engine运行边界/tests/engine_boundary_test.sh`，确认当前实现失败于结构边界缺失。
- [x] 创建 `state_model.go` 并移动持久化状态模型。
- [x] 创建 `engine_run.go` 并移动 Engine 构造、提交和运行循环。
- [x] 创建 `engine_resume.go` 并移动恢复和 lock 策略。
- [x] 创建 `engine_stage.go` 并移动单阶段执行、选项解析和验证入口。
- [x] 创建 `engine_progress.go` 并移动 progress writer 和 session 持久化 helper。
- [x] 运行 `go test ./internal/app`、本提案契约测试和受影响 shell 合同。
