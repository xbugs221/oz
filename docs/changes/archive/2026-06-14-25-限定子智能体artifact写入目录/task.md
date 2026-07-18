# 任务

## 1. 先运行创建阶段契约测试

- [x] 1.1 运行 `bash docs/changes/archive/2026-06-14-25-限定子智能体artifact写入目录/tests/test_subagent_artifact_directory_contract.sh`。
- [x] 1.2 确认初始失败点是目标行为缺失：member artifact 仍是单 JSON 路径、prompt 仍要求最终裸 JSON、或 CLI 缺少 `validate-member-artifact`。
- [x] 1.3 保存运行日志到 `test-results/25-subagent-artifact-directory/subagent-artifact-directory.log`。

## 2. 实现 artifact 目录路径

- [x] 2.1 调整 `memberArtifactPath`，让每个 member 使用 `<member-slug>.artifact/member.json`。
- [x] 2.2 确认 fan-in、status、go-dag node artifact 都继续通过 `memberArtifactPath` 获取路径。
- [x] 2.3 确认不同 member、不同 group、不同 iteration 不会共用 artifact 目录。

## 3. 更新 subagent prompt 和读取顺序

- [x] 3.1 在 prompt 中加入 `ARTIFACT_DIR`、`ARTIFACT_PATH` 和 `wo validate-member-artifact` 命令。
- [x] 3.2 要求 subagent 写文件并自行校验；最终回复不再承担 JSON 传输职责。
- [x] 3.3 `nodeRunSubagent` 优先读取已存在的 `ARTIFACT_PATH`，不存在时再 fallback 到最终回复捕获。
- [x] 3.4 retry prompt 必须复用同一 artifact 路径，并把上次 CLI/schema 错误反馈给 subagent。

## 4. 新增 CLI 校验命令

- [x] 4.1 新增 `wo validate-member-artifact --artifact <path> --group <group> --member <member> --change <change-name>`。
- [x] 4.2 成功时输出包含 artifact 路径、member 名称和规范化 status 的通过信息。
- [x] 4.3 失败时输出字段路径、期望类型、实际类型和修复建议，至少覆盖 `evidence` 非字符串数组的错误。
- [x] 4.4 复用现有 member artifact schema 和 change_name 校验，不另起一套格式。

## 5. 保持写边界

- [x] 5.1 调整 subagent 前后 git snapshot 检查，只允许当前 member 的 artifact 目录产生持久变化。
- [x] 5.2 sibling member artifact、当前提案文件、源码文件或其它 run 文件变化仍必须阻断。
- [x] 5.3 保留最终回复捕获 fallback，避免 Pi read-only backend 立即不可用。

## 6. 验证

- [x] 6.1 重新运行 `bash docs/changes/archive/2026-06-14-25-限定子智能体artifact写入目录/tests/test_subagent_artifact_directory_contract.sh`。
- [x] 6.2 运行 `go test ./internal/app -run 'Subagent|Parallel|Validate' -count=1`。
- [x] 6.3 运行 `oz validate 25-限定子智能体artifact写入目录 --json`。

## 7. 收窄 helper gate 语义

- [x] 7.1 明确 QA/review/implementation helper 是主阶段证据输入，不是单个 helper artifact hard gate。
- [x] 7.2 `nodeRunSubagent` 在 artifact schema/capture 重试耗尽后写入 failed member artifact，并让主流程继续。
- [x] 7.3 Review gate 不因 helper artifact 缺失、格式错误或 raw finding 覆盖主 reviewer 决策。
- [x] 7.4 QA gate 只在已有 helper artifact 报告当前提案 blocker/major finding 且主 QA 仍 clean 时阻断。
- [x] 7.5 implementation context 不因 required helper 失败阻断 execution，但写边界破坏仍阻断。
- [x] 7.6 补充 `TestSubagentMalformedArtifactBecomesAdvisoryInput` 回归测试。
