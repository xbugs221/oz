# 简报：运行态清理与测试基建治理

本提案把两个“后续重构安全性”问题合并成一个中型治理包：`oz flow clean` 需要先生成可复核计划再执行删除；核心 workflow 测试需要提取共享 fixture，降低后续改 Engine、DAG、acceptance gate 的测试维护成本。

交付目标是新增 clean plan/apply 边界、`oz flow clean --dry-run --json` 预览能力，并提取 workflow 测试夹具。执行阶段默认先阅读本目录的 `acceptance.json`、`tests/clean_plan_and_fixture_contract_test.sh`、`spec.md`，再阅读 `internal/app/clean.go` 和 `internal/app/go_dag_execution_context_test.go`。

非目标：不改变 clean 默认删除策略，不删除源码，不回滚 git，不把 shell specs 全部迁到 Go。

验收入口：`bash docs/changes/40-运行态清理与测试基建治理/tests/clean_plan_and_fixture_contract_test.sh`。该测试会写入 `test-results/40-clean-plan-fixture/contract.log`。
