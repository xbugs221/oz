# 任务

## 1. 先运行创建阶段契约测试

- [x] 1.1 运行 `bash docs/changes/36-清理历史垃圾并隐藏内部引擎信息/tests/test_no_internal_engine_user_surface_contract.sh`，确认当前实现因 `go-dag` 用户可见残留失败。
- [x] 1.2 运行 `bash docs/changes/36-清理历史垃圾并隐藏内部引擎信息/tests/test_no_legacy_root_tests_contract.sh`，确认当前实现因根历史 `tests/2026-*` 测试层失败。
- [x] 1.3 运行 `bash docs/changes/36-清理历史垃圾并隐藏内部引擎信息/tests/test_current_surface_cleanup_contract.sh`，确认当前实现因旧 `wo`/Dagu/legacy 维护面残留失败。

## 2. 隐藏内部引擎信息

- [x] 2.1 更新 `docs/specs/codex-workflow-cli/spec.md`，把 `go-dag` 和 engine 选择从用户合同中删除。
- [x] 2.2 更新 CLI 帮助、graph、status、watch、run 错误输出，不再输出 `go-dag`。
- [x] 2.3 删除默认配置和 profile 输出中的 engine 字段。
- [x] 2.4 保留内部 `go_dag` 实现命名，但不把它写入用户可见文案。

## 3. 清理根历史测试层

- [x] 3.1 归类 `tests/2026-*`：已覆盖的删除，仍有业务价值的迁移为当前 `tests/specs` 或 Go 测试。
- [x] 3.2 删除根测试层里的 `cmd/wo`、`wo.yaml`、`.wo`、`XDG_STATE_HOME/wo` 和旧 `wo` 命令引用。
- [x] 3.3 更新 `tests/specs/codex-workflow-cli/test_root_test_layout_contract.sh`，禁止 dated legacy shell 测试重新进入根目录。

## 4. 清理活跃维护面旧产品合同

- [x] 4.1 清理 `internal/` 中旧 `.wo` 路径、`WO_*` 产品变量和旧兼容注释。
- [x] 4.2 清理 `tests/specs` 中临时二进制变量 `WO_BIN`、`wo_bin`、`WO_TEST_*` 等旧命名。
- [x] 4.3 清理 Dagu、legacy-agent、opencode 的活跃合同残留；旧输入拒绝 fixture 必须窄白名单。
- [x] 4.4 更新 README、规格和发布门禁文案，当前产品只描述 `oz flow`。

## 5. 验证

- [x] 5.1 重新运行三个创建阶段契约测试并保存 `test-results/36-cleanup/` 日志。
- [x] 5.2 运行受影响 specs 测试。
- [x] 5.3 运行 `bash docs/changes/36-清理历史垃圾并隐藏内部引擎信息/tests/test_go_test_all_contract.sh`。
- [x] 5.4 人工核对 `docs/changes/archive/**` 没有被本次误清理。
