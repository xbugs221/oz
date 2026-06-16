# 提案：统一验收合同校验执行证据链

## 问题

`acceptance.json` 已经承担提案执行的硬合同，但它的生命周期被拆散在多个入口：

- `internal/acceptance` 校验 JSON shape 和 producer 追溯。
- `oz validate` 自己校验测试路径、evidence 路径和 runtime artifact policy。
- `oz flow` execution preflight 再做一次 producer 追溯。
- `oz flow run-acceptance` 负责执行 required tests 和检查 evidence 是否存在。
- QA gate 负责检查 acceptance matrix 覆盖。

这些职责各自正确，但没有一个共享的 lifecycle 结果对象。后续增加诊断、证据链说明或修复 producer 规则时，容易让不同入口输出不一致。

## 目标

- 在 `internal/acceptance` 中沉淀 lifecycle API，统一读取、校验、producer 追溯、required_tests 执行摘要和 QA 覆盖诊断。
- `oz validate` 复用 lifecycle diagnostics。
- `oz flow run-acceptance` 输出可复核的 diagnostics/coverage/producer 信息。
- execution preflight 与 QA gate 使用同一套 evidence producer 和 coverage 判断。
- 保持旧 `acceptance.json` schema 和现有 JSON result 字段兼容。

## 非目标

- 不增加 acceptance schema 新必填字段。
- 不删除 `run-acceptance` 现有 result 字段。
- 不改变 `required_tests[].command` 仍由 shell 执行的行为。
- 不把 QA artifact schema 合并进 acceptance schema。

## 验收

创建阶段契约测试必须通过：

```bash
bash docs/changes/39-统一验收合同校验执行证据链/tests/acceptance_lifecycle_contract_test.sh
```

执行阶段还必须运行：

```bash
go test ./internal/acceptance ./internal/ozcli ./internal/app
go test ./...
```
