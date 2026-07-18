# 任务：彻底合并 wo 为 oz flow

## 1. 先运行创建阶段合同测试

- [x] 运行 `bash docs/changes/archive/2026-06-14-30-彻底合并wo为oz-flow/tests/test_single_oz_flow_binary_contract.sh`，确认初始失败点来自 `cmd/wo` 仍存在、`oz flow` 未实现或发布面仍引用 `wo`。
- [x] 运行 `bash docs/changes/archive/2026-06-14-30-彻底合并wo为oz-flow/tests/test_no_wo_legacy_surface_contract.sh`，确认初始失败点来自旧产品命名残留。

## 2. 合并命令入口

- [x] 在 `internal/ozcli` 增加 `flow` 命令组，把 `oz flow <args>` 分发到工作流应用层。
- [x] 删除 `cmd/wo`，确保仓库只剩 `cmd/oz` 一个 CLI 入口。
- [x] 更新 `oz --help` 和 `oz flow --help`，让日常入口清晰展示。

## 3. 清理历史命名

- [x] 用 `ruplacer` 批量把活跃源码、README、CI、specs、tests 和模板中的 `wo` 产品面替换为 `oz flow` 或 `oz-flow`。
- [x] 把配置文件名从 `wo.yaml` 改为 `oz-flow.yaml`。
- [x] 把运行态命名空间从 `wo` 改为 `oz/flow` 或等价的 `oz` 命名空间。
- [x] 重命名 `prompts-template/wo-*.md` 等模板文件，保持 embed 和生成逻辑可用。

## 4. 更新发布和长期规格

- [x] 更新 `.github/workflows/`，只构建和验证 `oz`。
- [x] 更新 README 和 `docs/specs/`，删除双二进制发布合同和旧 `wo` 用法。
- [x] 更新长期 specs/tests，使其通过 `oz flow` 真实入口验证工作流行为。

## 5. 验证

- [x] 运行 `bash docs/changes/archive/2026-06-14-30-彻底合并wo为oz-flow/tests/test_single_oz_flow_binary_contract.sh`。
- [x] 运行 `bash docs/changes/archive/2026-06-14-30-彻底合并wo为oz-flow/tests/test_no_wo_legacy_surface_contract.sh`。
- [x] 运行 `go test ./...`。
- [x] 运行 `oz validate 30-彻底合并wo为oz-flow --json`。
