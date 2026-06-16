# 设计：收敛工作流阶段状态和门禁流水线

## 当前结构

```text
runLoop
  -> checkStageArtifactGate
  -> runAcceptancePreflight
  -> runAcceptanceGate
  -> validateStage
  -> markStageCompleted
  -> advance

nodeRunStage
  -> nodeStageDone/checkStageArtifactGate
  -> runAcceptancePreflight
  -> runAcceptanceGate
  -> validateStage
  -> markStageCompleted
  -> advance
```

两条路径表达的是同一个业务事实：一个主阶段是否可以结束，以及结束后 durable state 应该如何变化。

## 目标结构

```text
workflow stage strings
  -> parseWorkflowStage / workflowStage helpers
  -> status compatibility helpers

runLoop
  -> completeMainStage

nodeRunStage
  -> completeMainStage

completeMainStage
  -> artifact gate
  -> acceptance preflight
  -> acceptance run
  -> validation commands
  -> mark completed
  -> advance
  -> return action/result
```

`completeMainStage` 不应直接承担调度；它只处理“当前主阶段完成一次尝试后的门禁和状态变更”。外层 loop 和 DAG node 仍负责何时运行 agent、何时重试、何时写 node result。

## 阶段和状态 helper

建议引入轻量内部类型，例如：

```text
workflowStage
  raw       execution | review_1 | qa_1 | fix_1 | archive
  kind      execution | review | qa | fix | archive
  iteration 0 | n

runStatus
  raw public JSON string
  helpers: running/done/terminal/blocked
```

这些类型只在 Go 内部使用。对外 JSON、已有测试 fixture 和历史 state 继续使用字符串。

## 门禁结果

流水线结果建议包含：

```text
stageGatePipelineResult
  completed bool
  blocked bool
  retry bool
  nodeStatus string
  reason string
```

外层调用者根据结果决定打印 progress、写 node result 或继续循环。

## 风险

- 最大风险是把原本 loop 和 DAG 细微差异误合并。缓解方式是先写合同测试，再逐步迁移，每一步都运行 `go test ./internal/app`。
- 历史 state 兼容必须保留。任何新增 helper 都必须接受旧字符串。
- 不应在本提案顺手重写 status view，否则风险会扩大。
