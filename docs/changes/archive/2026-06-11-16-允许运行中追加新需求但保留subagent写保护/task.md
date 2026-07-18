# 任务：允许运行中追加新需求但保留 subagent 写保护

## 1. 先运行创建阶段契约测试

- [x] 1.1 运行 `bash docs/changes/archive/2026-06-11-16-允许运行中追加新需求但保留subagent写保护/tests/test_running_demand_insertion_contract.sh`，确认当前实现因新增非当前 change 被误拦截而失败。
- [x] 1.2 运行 `bash docs/changes/archive/2026-06-11-16-允许运行中追加新需求但保留subagent写保护/tests/test_manual_intervention_docs_contract.sh`，确认主规格尚未描述新的保护边界。

## 2. 实现路径感知保护

- [x] 2.1 为 git status snapshot 增加路径分类，识别当前 change、非当前 change 和危险源码路径。
- [x] 2.2 修改 `detectManualIntervention`，允许纯非当前 change 变化继续，并更新 baseline。
- [x] 2.3 保留源码、配置、测试、当前 change 和主规格变化的中止语义。
- [x] 2.4 错误信息列出阻断路径示例，避免用户误解为禁止一切 git 状态变化。

## 3. 加强 subagent 写保护

- [x] 3.1 在 subagent 节点执行前后复用路径分类，确认只读 subagent 没有写入当前 run 相关路径或源码。
- [x] 3.2 subagent 违规写入时，节点失败并写明违规路径。
- [x] 3.3 保证用户新增非当前 change 不影响当前 run 的 prompt、acceptance 和 review 范围。

## 4. 文档和长期测试

- [x] 4.1 更新 `docs/specs/codex-workflow-cli/spec.md` 的人工干预保护描述。
- [x] 4.2 补充 `docs/changes/archive/2026-06-11-16-允许运行中追加新需求但保留subagent写保护/tests/app` 或 `tests/specs/codex-workflow-cli` 长期回归测试。
- [x] 4.3 运行本提案契约测试、对应长期测试和 `oz validate 16-允许运行中追加新需求但保留subagent写保护 --json`。
