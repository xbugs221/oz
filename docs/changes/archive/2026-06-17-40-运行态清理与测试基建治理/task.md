# 任务：运行态清理与测试基建治理

## 契约测试先行

- [x] 运行 `bash docs/changes/40-运行态清理与测试基建治理/tests/clean_plan_and_fixture_contract_test.sh`，确认失败点来自 dry-run plan 或 fixture 缺失。
- [x] 记录失败日志到 `test-results/40-clean-plan-fixture/contract.log`。
- [x] 运行 `go test ./internal/app`，确认基线。
- [x] 阅读 `internal/app/clean.go`。
- [x] 阅读现有 clean 相关 Go 测试。
- [x] 阅读 `internal/app/go_dag_execution_context_test.go` 中重复 fixture。

## clean plan/apply

- [x] 新增 clean plan 文件，说明文件功能目的。
- [x] 定义 `CleanPlan`。
- [x] 定义 run plan item。
- [x] 定义 batch plan item。
- [x] 定义 session plan item。
- [x] 实现 `BuildCleanPlan`，只扫描不删除。
- [x] 把 failed run 标记为 delete。
- [x] 把 blocked run 标记为 delete。
- [x] 把 interrupted run 标记为 delete。
- [x] 把 corrupt run 标记为 delete。
- [x] 把 running active lock run 标记为 protect。
- [x] 把 done/archived run 标记为 protect。
- [x] 把 failed batch 标记为 delete。
- [x] 把 active referenced run 的 batch 标记为 protect。
- [x] 收集 cleanable/protected agent sessions。
- [x] 实现 `ApplyCleanPlan`。
- [x] 让 `CleanRuntimeStateWithOptions` 使用 plan/apply。

## CLI dry-run

- [x] 扩展 clean options 支持 `--dry-run`。
- [x] 扩展 clean options 支持 `--json`。
- [x] 实现 `oz flow clean --dry-run --json`。
- [x] dry-run 不得删除 run 目录。
- [x] dry-run 不得删除 batch 目录。
- [x] dry-run 不得删除 agent session。
- [x] JSON 输出包含 action 和 reason。
- [x] 保持 `oz flow clean` 人类输出兼容。
- [x] 保持未知参数错误文案清晰。

## 测试夹具

- [x] 新增 workflow fixture 测试 helper 文件，说明文件功能目的。
- [x] 提取临时 git repo 创建 helper。
- [x] 提取 active change 写入 helper。
- [x] 提取 acceptance contract 写入 helper。
- [x] 提取 fake agent runner。
- [x] 提取 fake tool registry。
- [x] 提取常用 git helper。
- [x] 迁移至少一组 DAG/gate 测试使用新 fixture。
- [x] 保持测试断言写在具体测试中，不藏进 fixture。

## 测试和收尾

- [x] 新增 clean dry-run 不删除测试。
- [x] 新增 clean plan/apply 一致性测试。
- [x] 新增 active lock protect 测试。
- [x] 新增 corrupt state plan 测试。
- [x] 新增 workflow fixture 自测。
- [x] 运行创建阶段契约测试。
- [x] 运行 `go test ./internal/app`。
- [x] 运行 `go test ./...`。
- [x] 确认没有改变 clean 默认删除策略。
