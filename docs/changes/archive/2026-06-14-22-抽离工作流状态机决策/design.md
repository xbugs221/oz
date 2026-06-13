# 文件目的

本文件记录状态机决策抽离的技术方案。

## 技术方案

建议边界：

- 决策层输入：`State`、当前阶段 artifact 摘要、review/qa 结果摘要、workflow config。
- 决策层输出：`StageDecision`，包含 `NextStage`、`NextStatus`、`BlockedReason`、`NeedsRerun` 等字段。
- `Engine` 保留读取 review/qa 文件、保存 state、运行 agent、运行 validation 的职责。

第一步不追求完全纯化所有逻辑，优先抽 `advance` 的阶段跳转规则和 `artifactDone` 的阶段分派规则。

## 取舍

不引入状态机库。当前阶段数量少，Go 表驱动测试比框架更清晰。

## 风险

- 抽离过度会造成参数对象膨胀。缓解方式是按阶段输出小 DTO，不把整个 Engine 传入决策层。
- go-dag 与 legacy runLoop 可能存在细微差异。缓解方式是保留现有 go-dag execution context 测试，并新增等价测试。
