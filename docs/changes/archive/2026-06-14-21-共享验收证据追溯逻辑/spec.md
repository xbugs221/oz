# 文件目的

本文件定义共享验收 evidence producer 追溯后的可验收行为。

### 需求：验收 producer 追溯规则只有一个实现来源

#### 场景：`oz validate` 和 `wo` 预检复用 `internal/acceptance`

- 对应测试：`docs/changes/21-共享验收证据追溯逻辑/tests/shared-producer-contract_test.sh`
- 真实数据来源：当前源码中的 `cmd/oz`、`internal/app`、`internal/acceptance`。
- 入口路径：在仓库根目录执行脚本。
- 关键断言：`cmd/oz` 和 `internal/app` 不得继续定义本地 `acceptanceEvidenceHasProducer` 等重复追溯 helper；`internal/acceptance` 必须提供 producer/finding 追溯 API。
- 剩余风险：静态断言不能证明函数命名完全符合最终设计，但能阻止重复规则继续存在。

### 需求：共享逻辑保持现有合同强度

#### 场景：合法 producer 和缺失 producer 的行为都由测试覆盖

- 对应测试：`docs/changes/21-共享验收证据追溯逻辑/tests/shared-producer-contract_test.sh`
- 真实数据来源：新增或迁移的 `internal/acceptance` Go 单元测试。
- 入口路径：脚本执行 `go test ./internal/acceptance ./internal/app ./cmd/oz`。
- 关键断言：测试必须覆盖 metadata producer、test file producer、sibling shell wrapper producer 和 missing producer。
- 剩余风险：不要求本提案覆盖所有历史 acceptance fixture，只要求核心追溯路径不漂移。
