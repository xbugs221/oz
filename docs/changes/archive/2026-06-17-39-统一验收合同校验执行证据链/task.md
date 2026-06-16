# 任务：统一验收合同校验执行证据链

## 契约测试先行

- [x] 运行 `bash docs/changes/39-统一验收合同校验执行证据链/tests/acceptance_lifecycle_contract_test.sh`，确认失败点来自 lifecycle 边界缺失。
- [x] 记录失败日志到 `test-results/39-acceptance-lifecycle/contract.log`。
- [x] 运行 `go test ./internal/acceptance ./internal/ozcli ./internal/app`，确认基线。
- [x] 阅读 `internal/acceptance/acceptance.go`。
- [x] 阅读 `internal/ozcli/cmd_validate.go`。
- [x] 阅读 `internal/app/acceptance_preflight.go`。
- [x] 阅读 `internal/app/acceptance_run.go`。
- [x] 阅读 `internal/app/qa.go`。

## lifecycle 边界

- [x] 在 `internal/acceptance` 新增 lifecycle 文件，说明文件功能目的。
- [x] 定义 lifecycle 诊断结构。
- [x] 定义 lifecycle 结果结构。
- [x] 收敛 contract shape validate 调用。
- [x] 收敛 required_tests 文件路径校验。
- [x] 收敛 required_tests command 引用 path 校验。
- [x] 收敛 required_evidence 相对路径校验。
- [x] 收敛 evidence producer 追溯。
- [x] 收敛 coverage required_tests 引用检查。
- [x] 收敛 coverage required_evidence 引用检查。
- [x] 为 lifecycle 增加表驱动 Go 测试。

## CLI 和 workflow 入口

- [x] 让 `oz validate` 使用 lifecycle diagnostics。
- [x] 保持 `oz validate --json` 旧字段兼容。
- [x] 让 execution preflight 使用 lifecycle producer diagnostics。
- [x] 让 `run-acceptance` 结果包含 lifecycle diagnostics。
- [x] 保持 `AcceptanceRunResult` 现有字段不删除。
- [x] 让 QA acceptance matrix 校验复用 lifecycle required item set。
- [x] 保持 `validate-qa --json` 输出兼容。
- [x] 保持错误文案中文可读。

## required_tests 执行

- [x] 确认 required_tests 仍按声明顺序执行。
- [x] 确认失败测试不短路后续 required_tests。
- [x] 确认 runtime evidence missing 进入统一 diagnostics。
- [x] 确认 result.json 写入路径不变。
- [x] 确认 log path 清洗逻辑不回退。
- [x] 确认 path traversal 防护不回退。

## 测试和收尾

- [x] 新增 lifecycle producer 正例 Go 测试。
- [x] 新增 lifecycle producer 反例 Go 测试。
- [x] 新增 `oz validate` 诊断一致性测试。
- [x] 新增 `run-acceptance` diagnostics JSON 测试。
- [x] 新增 QA matrix required item set 测试。
- [x] 运行创建阶段契约测试。
- [x] 运行 `go test ./internal/acceptance`。
- [x] 运行 `go test ./internal/ozcli`。
- [x] 运行 `go test ./internal/app`。
- [x] 运行 `go test ./...`。
- [x] 检查 `acceptance.json` schema 未新增必填字段。
- [x] 检查 README 或 specs 是否需要补充 diagnostics 描述。
- [x] 确认没有修改归档提案内容。
