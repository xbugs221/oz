# 文件目的

本文件定义统一状态展示视图模型后的可验收行为。

### 需求：human status、watch 和 JSON 共享结构化视图来源

#### 场景：状态文本渲染从 `app.go` 迁出

- 对应测试：`docs/changes/23-统一状态展示视图模型/tests/status-view-contract_test.sh`
- 真实数据来源：当前 `internal/app` 状态视图和 batch/status 测试。
- 入口路径：在仓库根目录执行脚本。
- 关键断言：`app.go` 不得继续定义 `watchBatchStatusLines`、`watchRunStatusLines`、`runProposalStatusLines` 等文本拼接 helper；`internal/app` 必须存在 status render 源文件。
- 剩余风险：静态检查不规定最终函数私有/公开命名，但必须证明职责迁出。

### 需求：用户可见状态输出保持稳定

#### 场景：batch、single run、watch spinner 和 compact rows 回归测试通过

- 对应测试：`docs/changes/23-统一状态展示视图模型/tests/status-view-contract_test.sh`
- 真实数据来源：`internal/app` status view 测试和迁入后的 status/batch 测试。
- 入口路径：脚本执行定向 `go test`。
- 关键断言：状态展示测试必须覆盖 running batch、failed batch、single run、watch spinner、status view rows 和 JSON contract。
- 剩余风险：不覆盖用户终端宽度差异；本提案只保证文本合同和数据来源一致。
