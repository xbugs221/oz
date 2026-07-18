# 文件目的

本提案把 `internal/app/state.go` 中的工作流阶段决策抽成可测试的纯决策层，降低后续维护风险。

## 用户问题

`state.go` 同时处理持久化、锁、agent 执行、artifact gate、validation、prompt 和阶段推进。核心阶段决策夹在 IO 逻辑里，重构或修 bug 时容易误伤状态机语义。

## 交付目标

- 新增独立状态机决策文件，承载阶段完成判断和下一阶段选择的纯逻辑。
- `Engine` 继续负责 IO、锁和落盘。
- 保持现有 go-dag、validation、review/fix/qa/archive 行为不变。

## 非目标

- 不重写 go-dag scheduler。
- 不改变工作流阶段名称和 JSON 状态格式。
- 不引入外部状态机库。

## 验收入口

执行 `bash docs/changes/archive/2026-06-14-22-抽离工作流状态机决策/tests/stage-decision-contract_test.sh`。

## 执行阶段默认上下文

先读取 `internal/app/state.go` 中 `artifactDone`、`advance`、`validateStage`、`detectManualIntervention` 以及相关测试，再从最小纯决策边界开始抽离。
