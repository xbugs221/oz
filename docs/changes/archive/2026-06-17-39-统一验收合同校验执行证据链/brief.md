# 简报：统一验收合同校验执行证据链

本提案解决 `acceptance.json` 从创建校验、执行 required tests、preflight producer 追溯到 QA acceptance matrix 的生命周期分散问题。验收合同是 oz 提案质量的硬边界，但当前实现分布在 `internal/acceptance`、`internal/ozcli` 和 `internal/app` 多处。

交付目标是沉淀共享 acceptance lifecycle API，让 `oz validate`、`oz flow run-acceptance`、execution preflight 和 QA gate 复用一致的诊断、producer 追溯和结果摘要。执行阶段默认先阅读本目录的 `acceptance.json`、`docs/changes/archive/2026-06-17-39-统一验收合同校验执行证据链/tests/acceptance_lifecycle_contract_test.sh`、`spec.md`，再阅读 `internal/acceptance/acceptance.go`、`internal/ozcli/cmd_validate.go`、`internal/app/{acceptance_preflight.go,acceptance_run.go,qa.go,stage_artifact_gate.go}`。

非目标：不改变 `acceptance.json` schema，不放宽业务级断言，不引入新的测试框架。

验收入口：`bash docs/changes/archive/2026-06-17-39-统一验收合同校验执行证据链/tests/acceptance_lifecycle_contract_test.sh`。该测试会写入 `test-results/39-acceptance-lifecycle/contract.log`。
