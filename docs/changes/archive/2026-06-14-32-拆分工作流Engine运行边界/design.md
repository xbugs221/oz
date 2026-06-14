# 设计：拆分工作流 Engine 运行边界

## 技术决策

1. 保持 `internal/app` 同包拆分，不引入新 package。
2. `state_model.go` 放置 `State`、`DAGNodeState`、`ProcessState`、`StageTiming`、`LockInfo` 等持久化模型。
3. `engine_run.go` 放置 `NewEngine`、`Start`、`Submit`、`run`、`runLoop` 等运行编排。
4. `engine_resume.go` 放置 `Resume`、`ResumeAfterUserChoice`、`ResumeRunJSON`、`resume`、`resumeRun`。
5. `engine_stage.go` 放置 `runStage`、`stageOptionsForRun`、`validateStage` 等单阶段执行和验证入口。
6. `engine_progress.go` 放置 `stageProgressWriter`、`subagentProgressWriter`、session key 和 progress value 解析。

## 取舍

本次不抽象接口，不引入新的 engine 类型。先用文件边界降低认知负担，等后续行为变更出现真实重复时再考虑更深抽象。

## 风险

- 移动未导出函数时可能漏掉 init 变量或测试替身。
- progress writer 会触碰 session 持久化，必须继续覆盖中断、恢复和并行 helper 场景。
