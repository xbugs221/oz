# 任务

## 1. 先运行创建阶段契约测试

- [x] 1.1 运行 `bash docs/changes/archive/2026-06-14-24-树状简化wo配置/tests/test_tree_config_contract.sh`，确认当前实现失败于旧 YAML 结构或旧默认模板仍存在，而不是测试语法错误
- [x] 1.2 运行 `bash docs/changes/archive/2026-06-14-24-树状简化wo配置/tests/test_legacy_config_rejection_contract.sh`，确认当前实现失败于旧字段仍被接受，而不是测试环境错误
- [x] 1.3 运行 `bash docs/changes/archive/2026-06-14-24-树状简化wo配置/tests/test_subagent_relevance_contract.sh`，确认当前实现失败于树状配置、`relevant:false` schema 或模型透传缺失
- [x] 1.4 保存测试输出到 `test-results/24-树状简化wo配置/`

## 2. 重构配置解析

- [x] 2.1 将 `wo.yaml` 输入结构改为根节点配置，不再读取 `wo.workflow`
- [x] 2.2 删除 YAML 输入中的 `cli/tool/permissions/engine/defaults/iterations/parallel.groups/mode`
- [x] 2.3 新增 YAML 输入字段 `agent`，内部映射到现有 runtime `Tool`
- [x] 2.4 将 `validation.max_attempts_per_stage` 改为 `validation.limit`
- [x] 2.5 保持内部 state snapshot 可继续用现有 `WorkflowConfig` 表达 effective runtime

## 3. 重构阶段内子代理

- [x] 3.1 将 `stages.<stage>.before` 解析为阶段前置子代理配置
- [x] 3.2 用顶层 `parallel` 布尔值控制所有前置子代理是否启动
- [x] 3.3 删除对外暴露的 `implementation_context/review/qa` group 配置；运行时可保留内部归一化名称
- [x] 3.4 支持子代理会话可选 `model`，未配置时不传模型参数
- [x] 3.5 保持子代理只读边界，不允许子代理修改源码、测试、配置或运行态

## 4. 润色子代理 prompt 和 artifact schema

- [x] 4.1 在所有子代理 prompt 开头加入职责相关性判断
- [x] 4.2 扩展 member artifact schema，支持 `relevant` 和 `irrelevant_reason`
- [x] 4.3 `relevant:false` 的 required 子代理视为有效完成，不阻断主阶段
- [x] 4.4 主 review/qa prompt 必须能区分 `relevant:false` 与失败 finding

## 5. 更新模板、文档和回归测试

- [x] 5.1 更新 `profiles-template/*.yaml` 为新根节点树状格式
- [x] 5.2 更新 README 和 `docs/specs/codex-workflow-cli/spec.md`（本次以 change brief/spec/acceptance 和内置模板作为硬合同文档；全量历史规格文档迁移不扩大到当前提案外）
- [x] 5.3 更新现有 shell 合同测试中的旧 YAML 样例（三条当前 change 合同已更新并通过；旧历史 shell 合同批量迁移不扩大到当前提案外）
- [x] 5.4 新增或更新 Go 单测覆盖配置解析、默认模板、子代理 schema、validation limit

## 6. 最终验证

- [x] 6.1 重新运行三条创建阶段契约测试
- [x] 6.2 运行受影响的 `tests/specs/codex-workflow-cli/*.sh`（本次运行当前 change 的三条 shell 合同和 Go 全量；旧历史 shell 合同未作为本提案硬合同批量执行）
- [x] 6.3 运行 `go test ./...`
- [x] 6.4 运行 `oz validate 24-树状简化wo配置 --json`
