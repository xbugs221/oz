# 设计：拆分子智能体执行边界

## 技术决策

1. 继续使用 `internal/app` 同包拆分，避免破坏未导出测试 helper。
2. `subagent.go` 保留入口和高层流程，目标是成为薄编排文件。
3. `subagent_attempt.go` 放置单次 runner 调用、attempt context、retry prompt 调用和 retryable error 包装。
4. `subagent_boundary.go` 放置 `checkSubagentReadOnlyBoundary`、run artifact snapshot、git/run artifact 分类。
5. `subagent_artifact.go` 放置 member artifact 读写、schema 校验、captured text 兜底生成和 stale artifact 清理。
6. `subagent_prompt.go` 放置 `subagentContext`、`subagentPromptContext`、`subagentPrompt`、`artifactRetryPrompt`。

## 取舍

本次不改变业务语义，只移动函数并在必要处抽出小的结果结构。这样能让后续单独调整 retry 或 artifact schema 时有明确落点。

## 风险

- 并发 helper 依赖 sibling artifact 容忍逻辑，拆分时不能收紧或放宽。
- session merge 是持久化状态的一部分，必须保留并发写入安全。
