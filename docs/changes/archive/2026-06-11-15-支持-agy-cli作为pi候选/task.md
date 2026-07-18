# 任务：支持 agy CLI 作为 Pi 候选

## 1. 先运行创建阶段契约测试

- [x] 1.1 运行 `bash docs/changes/archive/2026-06-11-15-支持-agy-cli作为pi候选/tests/test_agy_config_preflight_contract.sh`，确认当前实现因不认识 agy 或未预检 agy 失败。
- [x] 1.2 运行 `bash docs/changes/archive/2026-06-11-15-支持-agy-cli作为pi候选/tests/test_agy_cli_args_contract.sh`，确认当前实现因缺少 agy runner 或参数映射失败。

## 2. 接入 agy tool

- [x] 2.1 新增 `AgyTool` / `AgyCLI`，实现 `AgentTool` 和 `AgentRunner`。
- [x] 2.2 在 `NewAgentRegistry()` 注册 `AgyTool`。
- [x] 2.3 更新 `validAgentTool()` 和 `requiredAgentTools()`，让配置和预检认识 `agy`。
- [x] 2.4 实现 agy planning/sealed 参数构造，覆盖 `--print`、`--prompt-interactive`、`--model`、`--conversation`、权限和 sandbox 映射。

## 3. 状态、配置和文档同步

- [x] 3.1 确认 session key 使用 `agy:<role>`，不复用 `pi:<role>`。
- [x] 3.2 更新主规格 `docs/specs/codex-workflow-cli/spec.md`，声明 agy 是 Pi 候选后端。
- [x] 3.3 补充根目录长期回归测试，覆盖 agy 配置、预检和参数映射。
- [x] 3.4 更新发布门禁或相关测试清单，防止 allowlist 回退到 codex/pi。

## 4. 验证

- [x] 4.1 重新运行本提案两条契约测试并通过。
- [x] 4.2 运行新增长期回归测试并通过。
- [x] 4.3 运行 `oz validate 15-支持-agy-cli作为pi候选 --json` 并通过。
