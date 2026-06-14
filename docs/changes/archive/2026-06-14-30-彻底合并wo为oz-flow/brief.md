# 彻底合并 wo 为 oz flow

本提案把当前独立 `wo` 工作流工具彻底并入 `oz`，形成唯一命令行程序 `oz`，工作流能力统一通过 `oz flow ...` 命令组访问。

交付目标：

- 仓库最终只保留 `cmd/oz` 一个 CLI 入口，CI 和 Release 只构建、测试、发布 `oz`。
- 原 `wo status`、`wo watch`、`wo run`、`wo config`、`wo clean` 等工作流命令统一迁移为 `oz flow status`、`oz flow watch`、`oz flow run`、`oz flow config`、`oz flow clean`。
- 移除历史兼容层：不保留 `cmd/wo`、`wo` 兼容别名、旧命令提示、旧安装文档或双二进制发布合同。
- 工作流配置、运行态目录、模板文件名、用户可见提示和活跃规格测试中的产品命名统一为 `oz flow`，不继续使用 `wo.yaml` 或 `$XDG_STATE_HOME/wo` 作为新合同。

非目标：

- 不保留旧 `wo` 命令的平滑迁移能力。
- 不兼容既有本地 `wo` 运行态或 `wo.yaml` 配置；这是个人工具的破坏性清理。
- 不改变工作流 DAG、agent 执行语义、验收门禁语义或状态展示业务内容。

验收入口：

- `bash docs/changes/30-彻底合并wo为oz-flow/tests/test_single_oz_flow_binary_contract.sh`
- `bash docs/changes/30-彻底合并wo为oz-flow/tests/test_no_wo_legacy_surface_contract.sh`
- `go test ./...`

执行阶段默认上下文：先读 `cmd/oz/main.go`、`cmd/wo/main.go`、`internal/ozcli/ozcli.go`、`internal/app/app.go`、`internal/app/command_dispatch.go`、`internal/app/state_store.go`、`.github/workflows/`、`README.md` 和 `docs/specs/`。实现时优先做命名和入口收敛，不重写工作流业务逻辑。
