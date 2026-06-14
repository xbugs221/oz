# 提案：拆分运行状态持久化边界

## 背景

`internal/app/state.go` 已经同时承担 durable state 定义、执行循环、prompt 渲染、session 记录、state JSON 持久化、run lock、git snapshot 和人工干预检测。文件体量接近两千行，修改任何一块业务都容易触碰无关逻辑。

## 变更

- 保留 `State`、`Engine` 和执行循环在 `state.go`，让文件继续表达 sealed run 的核心生命周期。
- 新增 state store 边界，承载 `saveState`、`loadState`、`mergeState`、state JSON 写入和 run id 校验。
- 新增 prompt context 边界，承载 `promptForStage`、prompt snapshot、template context 构造。
- 新增 git guard 边界，承载 `gitSnapshot`、人工干预路径分类和 git porcelain 解析。
- 新增 run lock 边界，承载 run lock 文件读写、锁状态判断和中止信号入口。

## 为什么

这些逻辑都是 sealed run 可靠性的核心，但它们的变化原因不同：持久化关注 JSON 合同，prompt 关注 agent 输入，git guard 关注人工干预边界，lock 关注进程所有权。拆开后，后续修状态机、状态展示或并行子代理时不需要反复阅读整份 `state.go`。

## 非目标

- 不修改 `State` 的 JSON schema。
- 不修改 `wo status --json` 输出。
- 不修改 run 目录位置、prompt 文件名、acceptance 快照路径。
- 不引入新 package；先在 `internal/app` 包内按文件边界拆分。
