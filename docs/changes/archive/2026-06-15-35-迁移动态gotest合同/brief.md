# 35-迁移动态gotest合同

当前仓库已经有长期 Go 回归测试，但 `tests/specs/codex-workflow-cli` 里仍有多处 shell 合同临时写入 `docs/changes/archive/2026-06-15-35-迁移动态gotest合同/tests/app/*.gotest`，再依赖 `docs/changes/archive/2026-06-15-35-迁移动态gotest合同/tests/app/migrated_app_suite_test.go` 复制源码到临时模块执行。这让测试入口分散，失败时定位成本高，也容易让根目录测试门禁漏掉真实覆盖关系。

本次交付目标是把动态 `.gotest` 合同迁移为长期 Go 测试或直接的 shell 业务合同，删除动态 runner 依赖。非目标是不改被测业务行为、不削弱任何已迁移合同断言。

执行阶段默认先运行 `bash docs/changes/archive/2026-06-15-35-迁移动态gotest合同/tests/gotest_migration_test.sh`，确认当前实现失败于仍存在动态 `.gotest` 机制，再完成迁移并让测试通过。验收证据写入 `test-results/35-gotest-migration/contract.log`。
