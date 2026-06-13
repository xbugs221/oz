# 文件目的

本文件拆解状态机决策抽离的执行步骤。

## 任务

- [x] 先运行 `bash docs/changes/22-抽离工作流状态机决策/tests/stage-decision-contract_test.sh`，确认当前缺少独立决策层。
- [x] 从 `state.go` 抽出阶段跳转 DTO 和纯函数。
- [x] 为执行、review clean、review needs fix、QA needs fix、fix limit、archive done 增加表驱动测试。
- [x] 将 `Engine.advance` 和相关分派改为调用决策层。
- [x] 确认 `state.go` 明显变薄，不再承载全部阶段规则。
- [x] 运行本提案脚本和 `go test ./internal/app`。

## 验证条件

- [x] 独立 stage decision 文件存在。
- [x] 关键状态机测试通过。
- [x] `state.go` 行数下降到可维护阈值以内。
