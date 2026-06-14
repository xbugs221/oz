# 设计：拆分运行状态持久化边界

## 文件边界

- `state.go`：保留 `State`、`Engine`、`Start/Resume/runLoop/runStage` 等生命周期编排。
- `state_store.go`：保存和读取 `state.json`，包含 `saveState`、`loadState`、`mergeState`、`writeStateFileLocked`、`validateRunID`。
- `run_lock.go`：保存和判断 run lock，包含 `acquireLock`、`lockFileStatus`、`AbortRun`、`ArchiveSupersededRun`、`interruptLockedRun`。
- `prompt_context.go`：渲染 sealed run prompt，包含 prompt snapshot、`promptTemplateContext` 和 `promptContext`。
- `git_guard.go`：采集 git 快照并分类人工干预，包含 `gitSnapshot`、`classifyGitSnapshotChangeWithAllowed` 及 porcelain 解析 helper。

## 关键取舍

本次不拆 package。`internal/app` 内已有大量未导出 helper 和状态常量，直接拆 package 会引入导出类型和循环依赖风险。文件级边界能先降低阅读和审查成本，同时保持行为面稳定。

## 风险

- `saveState` 会调用 `refreshStateProcesses`，移动时必须保留这个副作用，否则 status JSON 会丢失并行进程观测字段。
- `gitSnapshot` 的 unborn branch 中文错误是用户可见合同，不能退化成原始 git 错误。
- prompt snapshot 是 sealed run 不漂移的核心，不能回退到运行时重新读当前配置。

## 验证策略

合同测试先检查边界文件和 `state.go` 中职责迁出情况，再运行 `internal/app` 中覆盖状态机、Go DAG、人工干预和 acceptance preflight 的真实 Go 回归测试。
