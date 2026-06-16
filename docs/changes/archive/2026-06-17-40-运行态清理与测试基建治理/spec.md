## 验收矩阵

| 需求 | 场景 | required_tests | required_evidence | 剩余风险 |
| --- | --- | --- | --- | --- |
| clean 先计划后执行 | dry-run JSON 展示将删除和保护项且不删除文件 | clean-plan-fixture-contract | clean-plan-fixture-log | 不覆盖所有平台 lock 细节，平台差异由既有单测补充 |
| clean 默认行为兼容 | apply 使用同一计划删除 failed run | clean-plan-fixture-contract | clean-plan-fixture-log | 不覆盖 agent session SQLite 所有 schema |
| workflow 测试夹具收敛 | 核心 workflow 测试存在共享 fixture | clean-plan-fixture-contract | clean-plan-fixture-log | 不要求一次性迁移所有测试文件 |

### 需求：clean 先计划后执行

系统必须支持在删除运行态前生成可复核 clean plan，并通过 `--dry-run --json` 输出计划。

#### 场景：dry-run JSON 展示将删除和保护项且不删除文件

- **测试**：`docs/changes/40-运行态清理与测试基建治理/tests/clean_plan_and_fixture_contract_test.sh`
- **真实数据来源**：契约测试创建的临时真实 git 仓库和 XDG runtime state。
- **入口路径**：`oz flow clean --dry-run --json`。
- **关键断言**：输出包含 failed run 的 delete 计划；dry-run 后 run 目录仍存在。
- **剩余风险**：不验证 Windows lock 行为。

### 需求：clean 默认行为兼容

系统必须保持 `oz flow clean` 默认实际清理行为，只把内部实现改为 apply clean plan。

#### 场景：apply 使用同一计划删除 failed run

- **测试**：`docs/changes/40-运行态清理与测试基建治理/tests/clean_plan_and_fixture_contract_test.sh`
- **真实数据来源**：同一个临时 runtime state。
- **入口路径**：`oz flow clean`。
- **关键断言**：dry-run 不删除；实际 clean 删除同一个 failed run。
- **剩余风险**：agent session 外部存储仍由现有测试覆盖。

### 需求：workflow 测试夹具收敛

系统必须把核心 workflow 测试中重复的 fake runner、临时 repo、acceptance writer 和 git helper 提取为共享测试夹具。

#### 场景：核心 workflow 测试存在共享 fixture

- **测试**：`docs/changes/40-运行态清理与测试基建治理/tests/clean_plan_and_fixture_contract_test.sh`
- **真实数据来源**：`internal/app` 测试源码。
- **入口路径**：shell 契约测试静态检查并运行 `go test ./internal/app`。
- **关键断言**：存在 workflow fixture 测试 helper；核心 Go 测试继续通过。
- **剩余风险**：不强制迁移每一个旧 helper，只要求新的共享夹具覆盖后续核心测试。
