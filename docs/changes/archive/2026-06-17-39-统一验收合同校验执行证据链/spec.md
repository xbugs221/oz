## 验收矩阵

| 需求 | 场景 | required_tests | required_evidence | 剩余风险 |
| --- | --- | --- | --- | --- |
| 验收生命周期有共享诊断 | validate、preflight、run-acceptance 复用 lifecycle | acceptance-lifecycle-contract | acceptance-lifecycle-log | 不验证所有诊断 code，只验证共享边界和关键字段 |
| required_tests 执行结果可追溯 | run-acceptance 输出 producer 和 coverage 诊断 | acceptance-lifecycle-contract | acceptance-lifecycle-log | 不限制 diagnostics 的内部排序 |
| 旧合同兼容 | 旧 acceptance schema 不新增必填字段 | acceptance-lifecycle-contract | acceptance-lifecycle-log | 不覆盖所有归档提案，只构造一个真实临时 change |

### 需求：验收生命周期有共享诊断

系统必须在 acceptance 包内提供共享 lifecycle 诊断边界，让 `oz validate`、execution preflight 和 `run-acceptance` 使用同一套 producer/coverage 判断。

#### 场景：validate、preflight、run-acceptance 复用 lifecycle

- **测试**：`docs/changes/archive/2026-06-17-39-统一验收合同校验执行证据链/tests/acceptance_lifecycle_contract_test.sh`
- **真实数据来源**：当前源码和契约测试创建的临时真实 git 仓库。
- **入口路径**：shell 契约测试构建真实 `oz` 二进制并运行公开 CLI。
- **关键断言**：存在 lifecycle 诊断边界；`oz validate` 和 `oz flow run-acceptance` 能基于同一合同给出一致成功结果。
- **剩余风险**：不强制 lifecycle API 的导出命名，但必须能从源码和输出中证明共享边界。

### 需求：required_tests 执行结果可追溯

`oz flow run-acceptance --json` 必须保留现有 summary/tests/evidence 字段，并新增足够诊断信息说明 required evidence 和 required tests 的 producer/coverage 关系。

#### 场景：run-acceptance 输出 producer 和 coverage 诊断

- **测试**：`docs/changes/archive/2026-06-17-39-统一验收合同校验执行证据链/tests/acceptance_lifecycle_contract_test.sh`
- **真实数据来源**：临时 change 中的 shell 测试真实写入 runtime evidence。
- **入口路径**：`oz flow run-acceptance --change <change> --json`。
- **关键断言**：JSON 结果包含 `diagnostics` 或等价 lifecycle 字段，且结果中能看到 evidence producer/coverage 信息。
- **剩余风险**：不要求具体 diagnostics code 名称。

### 需求：旧合同兼容

系统不得为了 lifecycle 诊断而修改 `acceptance.json` 的必填 schema。

#### 场景：旧 acceptance schema 不新增必填字段

- **测试**：`docs/changes/archive/2026-06-17-39-统一验收合同校验执行证据链/tests/acceptance_lifecycle_contract_test.sh`
- **真实数据来源**：测试中创建的最小合法旧 schema acceptance。
- **入口路径**：`oz validate <change> --json`。
- **关键断言**：旧 schema 合法合同仍通过 validate；新增 diagnostics 只出现在执行结果或内部边界中。
- **剩余风险**：不验证所有历史归档提案。
