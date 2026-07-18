# 任务

## 1. 先运行创建阶段契约测试

- [x] 1.1 运行 `bash docs/changes/archive/2026-06-09-5-彻底清理-wo-legacy-并精简续轮提示词/tests/test_no_dagu_runtime_residue_contract.sh`，确认当前实现因 Dagu 运行时/规格/长期测试残留而失败
- [x] 1.2 运行 `bash docs/changes/archive/2026-06-09-5-彻底清理-wo-legacy-并精简续轮提示词/tests/test_prompt_legacy_removed_contract.sh`，确认当前实现仍接受 `writing` 和历史 prompt 快照回退而失败
- [x] 1.3 运行 `bash docs/changes/archive/2026-06-09-5-彻底清理-wo-legacy-并精简续轮提示词/tests/test_review_fix_resumed_prompt_compact_contract.sh`，确认当前 review/fix 续轮提示词仍重复首轮要求而失败

## 2. 删除 Dagu 运行时和当前维护面残留

- [x] 2.1 删除 `StartDaguJSON`、Dagu CLI lookup、Dagu YAML 写入和 Dagu log 路径
- [x] 2.2 将 `dagu.go` 中仍被 `go-dag` 需要的 stage/gate/fanin helper 移到非 Dagu 命名文件
- [x] 2.3 删除 Dagu YAML graph exporter 和对应结构体、测试引用
- [x] 2.4 删除隐藏 `wo node` 入口，`go-dag` 内部改为直接调用 Go helper
- [x] 2.5 更新 README、`docs/specs/codex-workflow-cli/spec.md` 和长期测试，当前维护面不再出现 Dagu 合同

## 3. 移除 prompt legacy 兼容

- [x] 3.1 配置读取拒绝 `wo.prompts.writing`
- [x] 3.2 配置读取拒绝 `wo.workflow.stages.writing`
- [x] 3.3 删除 `prompts.writing` 到 execution/fix 的映射
- [x] 3.4 删除 `stages.writing` 到 execution/fix 的映射
- [x] 3.5 sealed run 恢复只读取 `prompt-snapshot.yaml` 当前角色 key，不再读取 `runs/<run-id>/prompts/*.md`
- [x] 3.6 删除只用于备份兼容的 legacy role/stage 代码和测试期望

## 4. 精简 review/fix 续轮提示词

- [x] 4.1 调整 `wo-review.md`，首轮保留完整方法论、schema 和示例
- [x] 4.2 调整 `wo-review.md` 续轮，只保留当前轮次路径、上一轮 artifact、角色会话 key、输出位置和单 JSON 约束
- [x] 4.3 调整 `wo-fix.md`，首轮保留根因分析方法论
- [x] 4.4 调整 `wo-fix.md` 续轮，只保留当前 findings、角色会话 key、输出位置和验证摘要要求

## 5. 验证与归档准备

- [x] 5.1 运行本提案 `docs/changes/archive/2026-06-09-5-彻底清理-wo-legacy-并精简续轮提示词/tests/` 下三个契约测试并确认通过
- [x] 5.2 运行 `go test ./...`
- [x] 5.3 按业务逻辑把本提案测试合并进 `tests/specs/codex-workflow-cli/`
- [x] 5.4 更新 `docs/specs/codex-workflow-cli/spec.md`，记录唯一 `go-dag` 运行时、无 legacy prompt fallback、review/fix 续轮精简合同
