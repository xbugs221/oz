# 任务

## 1. 契约测试先行

- [x] 运行 `bash docs/changes/archive/2026-06-10-11-新增-MADA-工作流profiles/tests/test_profile_templates_externalized_contract.sh`，确认当前实现因缺少 `profiles-template/*.yaml` 和默认配置仍硬编码在 Go 中失败，而不是测试语法或环境错误。
- [x] 运行 `bash docs/changes/archive/2026-06-10-11-新增-MADA-工作流profiles/tests/test_mada_profiles_config_contract.sh`，确认当前实现因缺少外置 profile 模板和 `--profile` 行为失败，而不是测试语法或环境错误。
- [x] 运行 `bash docs/changes/archive/2026-06-10-11-新增-MADA-工作流profiles/tests/test_mada_profile_discovery_contract.sh`，确认当前实现因缺少 `--list-profiles` 和未知 profile 诊断失败。

## 2. 默认配置模板抽离

- [x] 新增类似 `prompts-template/` 的 `profiles-template/` 内置模板目录和 embed 包。
- [x] 将现有默认 `wo config` 输出语义迁移到 `profiles-template/default.yaml`。
- [x] 从 `internal/app/config.go` 移除默认 subagent 角色名、purpose 和 profile YAML 的字符串拼接硬编码。
- [x] 默认 `wo config` 不带 `--profile` 时仍生成既有默认 YAML，并能被 `LoadWorkflowConfig` 和 `wo graph` 正常读取。

## 3. Profile 模板与命令入口

- [x] 在配置模块中增加内置 profile registry，至少包含 `default`、`mada-code`、`mada-decision`、`mada-research`。
- [x] 将 `mada-code`、`mada-decision`、`mada-research` 分别维护为 `profiles-template/*.yaml`，不得在 Go 源码中拼接长 YAML。
- [x] 支持 `wo config --profile <name>` 写入对应 profile 的标准 `wo.yaml`。
- [x] 支持 `wo config --global --profile <name>` 写入对应 profile 的全局 `~/wo.yaml`。
- [x] 支持 `wo config --list-profiles` 输出 profile 名称、中文用途和适用场景。
- [x] 未知 profile 必须非零返回，并提示输入名和可用 profile 名称。

## 4. 配置语义

- [x] 三个 MADA profile 都必须启用 `parallel.enabled`。
- [x] 三个 MADA profile 都必须配置 `planning_context`、`implementation_context`、`review`、`qa` 四组。
- [x] `review` 和 `qa` 使用 `gate_input`，`planning_context` 和 `implementation_context` 使用 `advisory`。
- [x] `mada-decision` 必须包含决策评审角色：需求澄清员、约束建模员、候选方案研究员、反方评审员、运维部署评审员、学习路线评审员、证据审计员。
- [x] 默认 `wo config` 不带 `--profile` 时仍生成既有默认 YAML。

## 5. 验证

- [x] 运行本提案三个契约测试并通过。
- [x] 运行 `go test ./...` 并通过。
- [x] 运行 `go run ./cmd/oz validate 11-新增-MADA-工作流profiles --json` 并通过。
- [x] 保存测试日志到 `test-results/11-mada-profiles/`，供 review/QA 复核。
