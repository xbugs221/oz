# 文件目的

本文件定义迁移测试合同收敛后的可验收行为。

### 需求：根测试门禁代表当前真实业务合同

#### 场景：根目录测试不再受过期 `.gotest` 迁移合同影响

- 对应测试：`docs/changes/20-收敛迁移测试合同/tests/migrated-tests-contract_test.sh`
- 真实数据来源：当前仓库的 `tests/app/`、`internal/app/`、`cmd/oz/` 和 `tests/` Go 测试包。
- 入口路径：在仓库根目录执行脚本。
- 关键断言：仓库不得继续保留 `tests/app/*.gotest` 作为根门禁输入；`go test ./...` 必须通过；现有真实包测试必须继续运行。
- 剩余风险：脚本不能证明每个历史断言都被语义等价迁移，只能阻止迁移层继续隐藏在根门禁中。

### 需求：后续重构拥有稳定测试基线

#### 场景：真实测试包可单独证明核心 app 和 oz CLI 入口仍可用

- 对应测试：`docs/changes/20-收敛迁移测试合同/tests/migrated-tests-contract_test.sh`
- 真实数据来源：`internal/app`、`cmd/oz` 和根测试包。
- 入口路径：同一脚本内执行定向 Go 测试。
- 关键断言：`go test ./internal/app ./cmd/oz ./tests` 必须通过，说明核心入口不是靠删除根门禁制造通过。
- 剩余风险：该脚本不替代全部业务 shell 测试，CI 仍需继续运行根目录 `tests/*.sh`。
