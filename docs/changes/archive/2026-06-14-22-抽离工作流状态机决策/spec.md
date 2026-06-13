# 文件目的

本文件定义状态机决策抽离后的可验收行为。

### 需求：阶段跳转规则可独立测试

#### 场景：执行、审核、修正、QA、归档阶段的下一步由决策层表达

- 对应测试：`docs/changes/22-抽离工作流状态机决策/tests/stage-decision-contract_test.sh`
- 真实数据来源：当前 `internal/app` 状态机测试和新增表驱动测试。
- 入口路径：在仓库根目录执行脚本。
- 关键断言：源码必须存在独立 stage decision 文件，包含 `StageDecision` 类型和 `DecideNextStage` 或等价函数；相关 Go 测试必须通过。
- 剩余风险：静态函数名检查可能需要执行阶段按最终命名微调脚本，但不能放弃独立决策层。

### 需求：抽离不改变工作流可观察行为

#### 场景：go-dag、validation、artifact gate 和 review/fix/qa 流程测试继续通过

- 对应测试：`docs/changes/22-抽离工作流状态机决策/tests/stage-decision-contract_test.sh`
- 真实数据来源：`internal/app` 现有 Go 测试。
- 入口路径：脚本执行定向 `go test`。
- 关键断言：`TestEngineStartRunsCleanReviewsToDone`、`TestQAFailureReturnsToFix`、`TestGoDAG*`、`TestValidationGate*` 等关键路径继续通过。
- 剩余风险：不覆盖外部 agent 真实执行，只覆盖已有 fake runner 和状态文件行为。
