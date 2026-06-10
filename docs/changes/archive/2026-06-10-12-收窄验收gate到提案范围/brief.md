# 简报

本次变更解决 oz/wo 提案执行中验收 gate 容易把无关历史债务纳入硬阻断的问题。目标是让单次提案按当前 `acceptance.json`、当前 diff 和当前变更触达路径收敛，同时仍保留对当前变更引入回归、权限风险和验收合同缺口的严格阻断。

交付目标：

- review、QA 和 parallel gate 能区分当前提案范围内问题与既有历史债务。
- 当前提案范围内的 blocker/major finding 继续阻断 clean。
- 既有历史债务可记录为非阻断 finding，不再导致当前提案迟迟无法完成。
- QA `acceptance_matrix` 继续严格只覆盖 `acceptance.json` 中定义的 id，不允许新增无关验收项。
- 已创建但尚未运行的旧提案不需要迁移，继续通过 `oz validate` 并按旧合同启动。

非目标：

- 不修复任何具体历史债务。
- 不取消 review、QA 或 parallel gate。
- 不放宽 `acceptance.json` 的严格校验。
- 不允许把当前变更引入的问题伪装成历史债务。
- 不批量修改已有 active proposal 的合同和测试。

验收入口：

- `bash docs/changes/12-收窄验收gate到提案范围/tests/test_review_non_blocking_debt_contract.sh`
- `bash docs/changes/12-收窄验收gate到提案范围/tests/test_parallel_scope_gate_contract.sh`
- `bash docs/changes/12-收窄验收gate到提案范围/tests/test_qa_acceptance_scope_contract.sh`
- `bash docs/changes/12-收窄验收gate到提案范围/tests/test_legacy_active_change_compatibility_contract.sh`
- `bash docs/changes/12-收窄验收gate到提案范围/tests/test_prompt_scope_contract.sh`

执行阶段默认上下文：

先读 `internal/app/review.go`、`internal/app/qa.go`、`internal/app/parallel.go`、`prompts-template/wo-review.md`、`prompts-template/wo-qa.md` 和 `docs/specs/codex-workflow-cli/spec.md` 中 review/QA/parallel artifact gate 合同。核心实现应收敛到 finding scope 和非阻断 finding 的统一模型，避免只靠提示词约束范围。
