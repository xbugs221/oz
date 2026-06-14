# 拆分工作流配置解析边界

本提案把 `internal/app/config.go` 中的配置 schema、profile 模板、parallel 展开和 validation 配置解析拆成独立边界，降低配置演进和旧字段拒绝逻辑的维护成本。

交付目标：

- `config.go` 保留公开配置入口和核心类型，细节解析拆到专门文件。
- profile 渲染、parallel/stages.before 展开、validation 配置、legacy 字段拒绝各自有清晰位置。
- 默认 `wo config`、profile 列表、legacy 配置拒绝、parallel 开关和 tree config 行为保持不变。

非目标：

- 不改变 `wo.yaml` 语义。
- 不新增 profile。
- 不修改 prompt 模板内容。

验收入口：

- `bash docs/changes/29-拆分工作流配置解析边界/tests/test_workflow_config_boundary_contract.sh`
- `go test ./internal/app -run 'Test.*Config|Test.*Profile|Test.*Parallel|Test.*Mada|Test.*Legacy' -count=1`

执行阶段默认上下文：先读 `internal/app/config.go`、`profiles-template/*.yaml`、`tests/specs/codex-workflow-cli/test_tree_config_contract.sh` 和 legacy/profile 相关 specs，保持配置业务合同优先。
