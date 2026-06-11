# 设计

## 当前路径

```text
go-dag
  -> implementation_context members
  -> implementation_context fan-in
  -> execution node
       -> artifactDone(execution)
          -> ChangeTasksDone
          -> ValidateParallelContextGate
  -> review / qa / archive
```

问题在于 task 完成度判断发生在 execution node 内部，而 execution node 的依赖已经要求执行前 subagents 和 fan-in 先完成。

## 目标路径

```text
go-dag
  -> execution readiness check
       | tasks done
       |   -> mark execution context skipped
       |   -> advance to review / qa / archive
       |
       | tasks pending
       |   -> implementation_context members
       |   -> implementation_context fan-in
       |   -> execution node
  -> review / qa / archive
```

实现时可以选择在 DAG 节点执行层增加 skip 判定，也可以在构图时引入 execution readiness 节点。无论采用哪种方式，公开行为必须满足：

- task 已全部完成时，不启动 execution 前 advisory subagent runner。
- task 已全部完成时，不要求存在 `parallel-implementation-context.json`。
- task 未完成时，仍要求 subagent artifact 和 fan-in gate 正常生成。

## 完成定义

本次只使用既有 `ChangeTasksDone(repo, changeName)` 定义执行任务完成：

```text
tasks.total > 0 && tasks.done == tasks.total
```

如果 task 总数为 0，按未完成处理，让既有 validation 继续暴露提案合同问题。

## 风险

- 如果实现只在 `nodeRunStage` 内跳过 execution，但不跳过前置 subagents，仍会浪费资源。契约测试会用 fake subagent 直接失败来覆盖这个风险。
- 如果实现粗暴关闭 `implementation_context`，未完成 task 的正常执行路径会失去上下文支持。第二个契约测试会要求未完成路径仍启动 subagents。
- 本变更不判断代码是否真的实现，只尊重 oz task 状态；review/QA 继续负责行为验收。

