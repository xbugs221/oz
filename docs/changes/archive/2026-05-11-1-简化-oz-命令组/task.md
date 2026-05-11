## 1. 测试

- [x] 1.1 在 ASCII `tests/` 包路径中新增真实 Go 测试，覆盖帮助输出区分 `list` / `install` 日常命令和 `status` / `validate` / `archive` 自动化接口；`docs/changes/1-简化-oz-命令组/tests/` 保留非 `.go` 测试来源说明，避免 `go test ./...` 被中文 import path 阻断。
- [x] 1.2 新增真实 Go 测试，覆盖 `oz list --json` 与 `oz l --json` 输出一致且只包含活动提案。
- [x] 1.3 新增真实 Go 测试，覆盖 `oz install` 本地安装、`oz install --global`、`oz i --global` 和 `oz i -g` 全局安装。
- [x] 1.4 新增真实 Go 测试，覆盖 `oz status --json`、`oz validate --json` 和 `oz archive --yes` 继续满足下游自动化行为。
- [x] 1.5 新增真实 Go 测试，覆盖 `init/create/exec/plan` 等阶段入口失败。

## 2. 实现

- [x] 2.1 将 `initCmd` 迁移为 `installCmd`，支持 `--global` 和 `-g`。
- [x] 2.2 收敛命令分发，允许 `list/l`、`install/i`、`status`、`validate`、`archive` 和元信息参数。
- [x] 2.3 增加顶层帮助、列表帮助、安装帮助和自动化接口帮助。
- [x] 2.4 删除或隔离 `init`、`plan`、`create`、`exec` 的用户入口，确保旧阶段命令不会继续执行。
- [x] 2.5 更新 README 中的命令列表和使用流程说明。

## 3. 验证

- [x] 3.1 运行 `go test ./...`。
- [x] 3.2 运行 `oz validate 1-简化-oz-命令组 --json`，确认提案结构和测试目录有效。

## 历史测试更新

- `cmd/oz/main_test.go` 原有 `init`、`create` 阶段入口测试与本次“阶段入口由 skill 驱动、CLI 不再执行”的新意图冲突，已改为覆盖 `install/i`、`list/l`、自动化接口帮助和旧阶段命令失败。
