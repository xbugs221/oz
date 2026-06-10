# 任务：精简后端为 Codex/Pi 并迁移测试

## 1. 先运行契约测试

- [ ] 1.1 运行 `bash docs/changes/14-精简后端为-codex-pi-并迁移测试/tests/test_backend_removal_contract.sh`，确认当前实现因旧后端残留失败。
- [ ] 1.2 运行 `bash docs/changes/14-精简后端为-codex-pi-并迁移测试/tests/test_cli_preflight_contract.sh`，确认当前实现因未强制检查两个 CLI 或仍接受旧配置失败。
- [ ] 1.3 运行 `bash docs/changes/14-精简后端为-codex-pi-并迁移测试/tests/test_root_test_layout_contract.sh`，确认当前实现因 `internal/` 测试残留失败。
- [ ] 1.4 运行 `bash docs/changes/14-精简后端为-codex-pi-并迁移测试/tests/test_docs_release_gate_contract.sh`，确认当前文档和发布门禁尚未同步失败。

## 2. 删除旧后端

- [ ] 2.1 删除旧后端 runner、参数构造、JSONL session 解析、集成测试和单元测试。
- [ ] 2.2 收窄 agent registry 和配置校验，只支持 `codex`、`pi`。
- [ ] 2.3 删除状态展示、恢复、session 隔离和测试夹具中的旧后端 key。
- [ ] 2.4 清理帮助文案、配置示例、profiles、主规格、长期测试和归档材料中的旧后端描述。

## 3. 增加启动前 CLI 检查

- [ ] 3.1 sealed run 创建运行态前检查 `codex` 和 `pi` 是否都在 PATH 中。
- [ ] 3.2 任一缺失时输出明确安装提示。
- [ ] 3.3 缺失 CLI 时不得创建或污染 run state。

## 4. 迁移测试布局

- [ ] 4.1 将 `internal/**/_test.go` 迁移到根目录 `tests/`，按业务能力组织。
- [ ] 4.2 将依赖未导出函数的测试改成 CLI 或业务行为测试，避免扩大导出面。
- [ ] 4.3 更新发布门禁和长期规格测试入口，只依赖根目录 `tests/`。

## 5. 验证和归档准备

- [ ] 5.1 重新运行本提案全部契约测试。
- [ ] 5.2 运行受影响的根目录长期测试。
- [ ] 5.3 更新 `docs/specs/codex-workflow-cli/spec.md`。
- [ ] 5.4 确认 `acceptance.json` 中 required tests 和 evidence 均有真实输出。
