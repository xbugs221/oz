# 任务

## 1. 契约测试

- [x] 1.1 先运行 `bash docs/changes/12-收窄验收gate到提案范围/tests/test_review_non_blocking_debt_contract.sh`，确认当前失败原因是 review artifact 不支持 `non_blocking_findings` 或 finding scope。
- [x] 1.2 先运行 `bash docs/changes/12-收窄验收gate到提案范围/tests/test_parallel_scope_gate_contract.sh`，确认当前失败原因是 parallel finding 无法按 scope 区分阻断范围。
- [x] 1.3 先运行 `bash docs/changes/12-收窄验收gate到提案范围/tests/test_qa_acceptance_scope_contract.sh`，确认当前失败原因是 QA artifact 不支持非阻断历史债务，且未知 acceptance id 仍被拒绝。
- [x] 1.4 运行 `bash docs/changes/12-收窄验收gate到提案范围/tests/test_legacy_active_change_compatibility_contract.sh`，确认旧格式 active change 兼容。
- [x] 1.5 先运行 `bash docs/changes/12-收窄验收gate到提案范围/tests/test_prompt_scope_contract.sh`，确认当前 prompt 缺少 scope 合同。

## 2. Artifact schema

- [x] 2.1 为 `Finding` 增加可选 `scope` 字段，并集中校验允许值。
- [x] 2.2 为 review 和 QA artifact 增加可选 `non_blocking_findings`。
- [x] 2.3 clean review/QA 继续拒绝 blocking `findings`，但允许 scope 为 `out_of_scope_existing` 的 `non_blocking_findings`。
- [x] 2.4 缺省 scope 按 `current_change` 处理，保持旧 artifact 阻断语义。

## 3. Gate 逻辑

- [x] 3.1 更新 parallel review/QA gate，只让 hard-blocking scope 的 blocker/major finding 阻断 clean。
- [x] 3.2 保持成员失败为硬阻断。
- [x] 3.3 保持 QA `acceptance_matrix` 只能引用 acceptance 合同中已有 id。

## 4. Prompt 和文档

- [x] 4.1 更新 `prompts-template/wo-review.md`，要求先判断 scope，并把历史债务写入 `non_blocking_findings`。
- [x] 4.2 更新 `prompts-template/wo-qa.md`，要求 QA 不得把历史债务混入 `acceptance_matrix`。
- [x] 4.3 更新 `docs/specs/codex-workflow-cli/spec.md` 中 review/QA/parallel gate 合同。

## 5. 验证

- [x] 5.1 运行本 change 的全部 `tests/` 契约测试并通过。
- [x] 5.2 运行相关 Go 单元测试：`go test ./internal/app -run 'Review|QA|Parallel' -count=1`。
- [x] 5.3 运行 `go test ./internal/acceptance ./cmd/oz -count=1`。
- [x] 5.4 运行 `go run ./cmd/oz validate 12-收窄验收gate到提案范围 --json` 并通过。
