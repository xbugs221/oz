# 规格：迁移动态 gotest 合同

## 验收矩阵

| 需求 | 场景 | required_tests | required_evidence | 关键断言 |
| --- | --- | --- | --- | --- |
| 动态 gotest 合同迁移 | 仓库不再依赖临时 .gotest runner | gotest-migration | gotest-migration-log | 不再出现 `.gotest`、`OZ_MIGRATED_APP_RUN`、migrated runner，且 `go test ./...` 通过 |

### 需求：动态 gotest 合同迁移

仓库必须移除临时 `.gotest` 注入机制，把原动态合同迁移到长期可发现测试入口，避免测试覆盖隐藏在 shell 写文件流程中。

#### 场景：仓库不再依赖临时 .gotest runner

- 测试文件：`docs/changes/35-迁移动态gotest合同/tests/gotest_migration_test.sh`
- 真实数据来源：仓库当前 `tests/specs`、`tests/app`、`internal/app` 测试代码和完整 Go 测试入口。
- 入口路径：执行 shell 契约测试，内部扫描动态 `.gotest` 机制并运行 `go test ./...`。
- 关键断言：`tests/specs` 和 `tests/app` 不得再引用 `.gotest` 或 `OZ_MIGRATED_APP_RUN`；`tests/app/migrated_app_suite_test.go` 必须删除；完整 Go 测试必须通过。
- 剩余风险：该测试不能自动证明每个旧动态断言都一字不漏迁移，执行阶段需要在变更说明中列出迁移映射。
