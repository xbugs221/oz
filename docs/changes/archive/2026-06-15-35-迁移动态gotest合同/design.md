# 设计：迁移动态 gotest 合同

## 技术决策

1. 优先把 `.gotest` 中的 Go 测试内容移到 `internal/app/*_test.go`，保持包内未导出 helper 可测。
2. 对本来更适合 CLI 端到端的场景，保留 shell 合同，但不再生成 `.gotest`。
3. 删除 `docs/changes/archive/2026-06-15-35-迁移动态gotest合同/tests/app/migrated_app_suite_test.go`，让 `docs/changes/archive/2026-06-15-35-迁移动态gotest合同/tests/app` 不再作为临时 Go 注入目录。
4. 更新相关 shell 合同，直接运行长期 Go 测试或运行真实 CLI 命令。
5. 使用 `rg` 契约禁止新增 `.gotest`、`OZ_MIGRATED_APP_RUN` 和 migrated runner 引用。

## 取舍

本次迁移会触碰较多测试文件，但目标是测试结构清晰，不改生产行为。为了避免大爆炸，可以按原 shell 合同分组迁移，但最终必须清除动态 runner。

## 风险

- 迁移时可能漏掉原 `.gotest` 的某个断言。
- 长期 Go 测试如果拆分不当，可能失去包内 helper 访问能力。
