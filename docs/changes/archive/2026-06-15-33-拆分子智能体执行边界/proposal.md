# 提案：拆分子智能体执行边界

## 背景

subagent 是 Go DAG 并行上下文和审核辅助的重要执行单元。当前实现把执行尝试、artifact 处理、边界校验和 prompt 放在一个文件里，导致修改任一职责都需要理解完整流程。

## 目标

- 保留 `nodeRunSubagent` 作为薄入口。
- 抽出单次执行尝试和重试结果处理。
- 抽出 run artifact 和 git diff 只读边界校验。
- 抽出 member artifact 读写、规范化、兜底生成。
- 抽出 subagent prompt/context 组装。
- 保持现有 subagent 并发、session merge、artifact schema 和只读保护回归通过。

## 非目标

- 不改变 `ParallelMemberResult` 字段。
- 不修改 helper prompt 的业务要求。
- 不调整三次重试策略和超时时间。
