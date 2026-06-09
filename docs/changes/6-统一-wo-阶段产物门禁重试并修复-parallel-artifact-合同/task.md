# 任务

## 1. 先运行创建阶段契约测试，确认当前失败点

- [x] 1.1 运行 `bash docs/changes/6-统一-wo-阶段产物门禁重试并修复-parallel-artifact-合同/tests/test_stage_artifact_gate_retry_all_roles.sh`，确认当前实现会在 execution/fix/archive 等缺失产物场景直接 failed，而不是同会话 retry
- [x] 1.2 运行 `bash docs/changes/6-统一-wo-阶段产物门禁重试并修复-parallel-artifact-合同/tests/test_batch_continues_after_stage_artifact_repair.sh`，确认当前 batch 会被可修复 execution artifact 问题中断
- [x] 1.3 运行 `bash docs/changes/6-统一-wo-阶段产物门禁重试并修复-parallel-artifact-合同/tests/test_parallel_subagent_info_severity_contract.sh`，确认当前 parallel member `severity: info` 会导致 workflow failed
- [x] 1.4 运行 `bash docs/changes/6-统一-wo-阶段产物门禁重试并修复-parallel-artifact-合同/tests/test_stage_prompt_contract_completeness.sh`，确认内置阶段提示词不会把首轮 execution/write 压缩成只读 `state.json` 和调用技能

## 2. 统一主阶段 artifact gate

- [x] 2.1 抽出阶段产物期望和检查函数，覆盖 execution、review_N、qa_N、fix_N、archive
- [x] 2.2 将 `done=false` 的缺失产物转成 `stageArtifactGateError`，而不是直接 `failNodeState`
- [x] 2.3 让 `nodeRunStage`、`runLoop` 和 `advance` 对 artifact gate failure 使用同一 `recordStageArtifactGateFailure` 路径
- [x] 2.4 补强 `validationFailurePrompt`，明确目标阶段产物路径、缺失/非法原因和“只补写或改写当前阶段产物”
- [x] 2.5 确保同阶段重试复用原 role session，且最多 3 次后进入阻断状态

## 3. 修复 parallel member artifact 合同

- [x] 3.1 增加统一 member artifact parse/normalize/validate 入口
- [x] 3.2 将 `info/informational/note/warning` 等 severity alias 归一为 `minor`
- [x] 3.3 不可归一的 member artifact 语义错误必须 resume 同一 subagent session，只重写 `SUBAGENT_OUTPUT`
- [x] 3.4 `nodeRunSubagent` 和 `nodeFanin` 都只接受规范化后的 member artifact
- [x] 3.5 错误信息必须包含 group、member、字段、非法值和 artifact 路径

## 4. 更新公开规格和提示词

- [x] 4.1 更新 `docs/specs/codex-workflow-cli/spec.md`，记录任意主阶段 artifact gate retry、batch 等待 retry 和 parallel severity alias 合同
- [x] 4.2 更新 `prompts-template/wo-start.md`，确保 execution 首轮包含 change 文档、acceptance、tests、required_tests、禁止改弱合同和 `oz status` task 完成标准
- [x] 4.3 更新 `prompts-template/wo-review.md`、`wo-qa.md`、`wo-fix.md`、`wo-done.md`，确保首轮包含对应输入 artifact、目标输出 artifact、禁止事项、验收/证据边界
- [x] 4.4 确认 review/QA/fix 的续轮只省略 JSON 示例和方法论长文，不省略当前目标 artifact、role session、上一轮必要引用或结构化输出要求
- [x] 4.5 更新 `prompts-template/wo-start.md`、`wo-review.md`、`wo-qa.md`、`wo-discuss.md` 中 parallel artifact 字段要求
- [x] 4.6 确认 retry prompt 不引导 agent 修改 acceptance 合同、其他阶段 artifact 或非目标文件

## 5. 验证

- [x] 5.1 重新运行本提案四条 shell 契约测试并保存 runtime log
- [x] 5.2 运行 `go test ./...`
- [x] 5.3 运行 `oz validate 6-统一-wo-阶段产物门禁重试并修复-parallel-artifact-合同 --json`
- [x] 5.4 人工核对 `acceptance.json`、`spec.md` 和测试脚本路径、断言、证据 ID 一致
