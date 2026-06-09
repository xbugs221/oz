# 任务

## 1. 契约测试基线

- [x] 1.1 先运行 `docs/changes/8-强化验收硬合同并精简执行上下文/tests/test_strict_acceptance_contract_validation.sh`，确认当前失败原因是 `oz validate` 尚未拒绝缺断言或弱断言合同。
- [x] 1.2 先运行 `docs/changes/8-强化验收硬合同并精简执行上下文/tests/test_execution_prompt_hard_contract_focus.sh`，确认当前失败原因是 execution prompt 仍默认要求读取全量长文档。
- [x] 1.3 先运行 `docs/changes/8-强化验收硬合同并精简执行上下文/tests/test_status_hard_contract_summary.sh`，确认当前失败原因是 `oz status --json` 尚未暴露 `brief.md` 和验收合同摘要。

## 2. 实现

- [x] 2.1 更新 `acceptance` 校验逻辑，要求 `required_tests[].assertions` 非空并拒绝明确弱断言。
- [x] 2.2 更新 `oz validate`，交叉校验测试路径、测试命令、`brief.md` artifact 和验收矩阵引用。
- [x] 2.3 更新内置 execution prompt，让默认上下文聚焦 `brief.md`、`acceptance.json` 和 `tests/`，长文档改为按需读取。
- [x] 2.4 更新 `oz status --json`，在 artifacts 中列出 `brief.md`，并输出验收合同摘要。

## 3. 验证

- [x] 3.1 重新运行本提案 `tests/` 下全部契约测试。
- [x] 3.2 运行相关 Go 单元测试和现有规格测试，确认没有破坏旧的 acceptance、prompt、status 合同。
- [x] 3.3 若旧测试与新意图冲突，按业务意图更新根目录历史测试，并在 `design.md` 记录原因。
