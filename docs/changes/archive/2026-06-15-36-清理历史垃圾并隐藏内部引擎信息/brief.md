# 清理历史垃圾并隐藏内部引擎信息

当前仓库已经收敛为 `oz flow` 工作流，但活跃维护面仍残留旧 `wo` 测试层、旧状态目录和旧配置叙述。另一个更直接的问题是：`go-dag` 只是内部实现细节，却仍在规格、测试或命令输出中被描述为用户需要理解的引擎名称。

交付目标：

- 非开发用户可见的帮助、配置、状态、graph、错误输出和文档中不再出现 `go-dag`、Dagu 或“engine”选择概念。
- 根目录历史 `tests/2026-*` shell 测试不再作为活跃测试层保留；仍有价值的业务场景迁移到当前 `tests/specs` 或 Go 测试。
- 活跃源码、规格、测试和模板不再保留旧 `wo` 产品面、旧 `.wo` 运行态、`wo.yaml`、独立 `cmd/wo` 或 legacy-agent/opencode 迁移残留。
- 保留必要的“旧输入被拒绝”合同，但这些合同只能证明旧格式失败，不能把旧格式写成当前用户合同。

非目标：

- 不改变工作流状态机、agent 执行、review/QA/fix/archive 语义。
- 不删除 `go_dag.go` 等开发者内部实现文件名，只清理用户可见产品面和活跃合同面。
- 默认不清理 `docs/changes/archive/**` 历史归档提案；归档文档可以继续记录历史决策。

验收入口：

- `bash docs/changes/36-清理历史垃圾并隐藏内部引擎信息/tests/test_no_internal_engine_user_surface_contract.sh`
- `bash docs/changes/36-清理历史垃圾并隐藏内部引擎信息/tests/test_no_legacy_root_tests_contract.sh`
- `bash docs/changes/36-清理历史垃圾并隐藏内部引擎信息/tests/test_current_surface_cleanup_contract.sh`
- `bash docs/changes/36-清理历史垃圾并隐藏内部引擎信息/tests/test_go_test_all_contract.sh`

执行阶段默认上下文：先读 `README.md`、`docs/specs/codex-workflow-cli/spec.md`、`tests/specs/codex-workflow-cli/test_no_wo_legacy_surface_contract.sh`、`tests/specs/codex-workflow-cli/test_root_test_layout_contract.sh`、`internal/app/command_dispatch.go`、`internal/app/graph.go`、`internal/app/config.go`、`internal/app/state_store.go` 和根目录 `tests/`。清理时优先迁移仍有业务价值的当前合同，再删除过期历史脚本。
