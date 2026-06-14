# 任务

- [ ] 1.1 先运行 `bash docs/changes/29-拆分工作流配置解析边界/tests/test_workflow_config_boundary_contract.sh`，确认失败点是配置边界文件缺失或 `config.go` 仍承载拆分职责。
- [ ] 1.2 新增 `config_schema.go`，迁出 YAML input 类型、严格解码、legacy root 检测和 input 合并。
- [ ] 1.3 新增 `config_profiles.go`，迁出 profile registry、profile YAML 渲染和 prompt 注入。
- [ ] 1.4 新增 `config_parallel.go`，迁出 parallel/subagents/stages.before 展开与校验。
- [ ] 1.5 新增 `config_validation.go`，迁出 validation 命令解析、复制和 limit 规范化。
- [ ] 1.6 运行配置合同脚本和 `go test ./internal/app -run 'Test.*Config|Test.*Profile|Test.*Parallel|Test.*Mada|Test.*Legacy' -count=1`。
- [ ] 1.7 人工核对默认 `wo.yaml`、legacy 诊断和 profile 列表输出没有行为变化。
