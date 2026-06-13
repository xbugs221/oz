# 文件目的

本文件说明抽离工作流状态机决策的动机和交付内容。

## 背景

后续重构需要在不改变 `wo` 业务行为的前提下降低复杂度。当前最大风险点是 `internal/app/state.go`，它既是运行态模型，又是状态机，又是持久化和 prompt 逻辑入口。

## 问题

阶段决策分散在 `artifactDone`、`advance`、`runLoop` 和 go-dag 节点中。任何小改动都可能影响 review/fix/qa/archive 跳转、validation retry 或 artifact gate 语义。

## 变更

- 新建状态机决策模块，例如 `internal/app/stage_decision.go`。
- 定义明确 DTO，例如 `StageDecision`，表达当前阶段是否完成、是否需要 gate retry、下一阶段或终态。
- 给纯决策函数补充表驱动测试。
- `state.go` 只调用决策层并负责 IO 副作用。

## 稳定性原则

先搬移可证明等价的逻辑，再做命名和局部整理。不得同时改变状态 JSON、stage 名称或 retry 次数语义。
