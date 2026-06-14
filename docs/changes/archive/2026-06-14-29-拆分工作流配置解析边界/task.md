# 任务

- [x] 1.1 先运行 `bash docs/changes/29-拆分工作流配置解析边界/tests/test_workflow_config_boundary_contract.sh`，确认失败点是配置边界文件缺失或 `config.go` 仍承载拆分职责。
- [x] 1.2 新增 `config_schema.go`，迁出 YAML input 类型、严格解码、legacy root 检测和 input 合并。
- [x] 1.3 新增 `config_profiles.go`，迁出 profile registry、profile YAML 渲染和 prompt 注入。
- [x] 1.4 新增 `config_parallel.go`，迁出 parallel/subagents/stages.before 展开与校验。
- [x] 1.5 新增 `config_validation.go`，迁出 validation 命令解析、复制和 limit 规范化。
- [x] 1.6 运行配置合同脚本和 `go test ./internal/app -run 'Test.*Config|Test.*Profile|Test.*Parallel|Test.*Mada|Test.*Legacy' -count=1`。
- [x] 1.7 人工核对默认 `wo.yaml`、legacy 诊断和 profile 列表输出没有行为变化。

执行记录：

- 历史测试 `tests/specs/codex-workflow-cli/test_mada_profiles_config_contract.sh` 仍断言 profile 模板包含旧根节点 `wo:` 和旧 `parallel.groups.enabled/tool` 输出，与当前 legacy 拒绝合同和本提案根节点 tree config 形态冲突；已改为断言 `stages:`、`parallel: true`、`stages.<stage>.before` 与 `agent: pi`，继续覆盖真实 profile 模板和 `wo graph` 可加载行为。
- 同一历史测试还断言 `mada-decision` 必须包含旧决策专用角色名，但当前内置模板使用通用 MADA 角色集；已改为核对当前模板里的 execution/review/qa 代表角色，避免把旧 profile 内容当作本提案阻塞项。
- 历史 parallel 合同的动态 Go 测试仍断言旧 OMO `parallel.groups.enabled` skeleton 和已不存在的角色名；已改为验证当前 `parallel: true`、`stages.<stage>.before` tree config 及当前内置角色。
