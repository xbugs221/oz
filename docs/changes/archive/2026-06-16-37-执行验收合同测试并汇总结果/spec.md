# 规格：执行验收合同测试并汇总结果

## 验收矩阵

| 需求 | 场景 | required_tests | required_evidence | 关键断言 |
| --- | --- | --- | --- | --- |
| 验收合同执行入口 | Runner surface 暴露 run-acceptance | acceptance-run-surface-contract | acceptance-run-surface-log | 帮助文本和 `contract --json` 都暴露 `run-acceptance` |
| 验收合同执行入口 | 成功执行 active change 的 required tests 并检查 evidence | acceptance-run-success-contract | acceptance-run-success-log | 真实临时项目中的 required test 被执行，JSON 显示 passed，日志和 evidence 落盘 |
| 验收合同执行入口 | 失败时仍汇总全部 required tests | acceptance-run-failure-contract | acceptance-run-failure-log | 一个测试失败时命令返回非零，但另一个测试仍执行并出现在 JSON 结果中 |
| 验收合同 gate 集成 | execution/fix 后使用同一执行器阻断失败合同 | acceptance-run-gate-contract | acceptance-run-gate-log | 存在独立 acceptance run 边界、stage gate 状态和对应 Go 回归测试 |
| 测试执行边界收敛 | acceptance run 不混入 validation 和 QA artifact 逻辑 | acceptance-run-gate-contract | acceptance-run-gate-log | `validation.go`、`qa.go` 不直接承载 required_tests 命令执行主逻辑 |

### 需求：验收合同执行入口

系统必须提供稳定命令执行 active change 的 `acceptance.json.required_tests`，并用机器可读 JSON 汇总测试与 evidence 状态。

#### 场景：Runner surface 暴露 run-acceptance

- 测试文件：`docs/changes/archive/2026-06-16-37-执行验收合同测试并汇总结果/tests/test_acceptance_run_contract_surface.sh`
- 真实数据来源：当前仓库 `cmd/oz` 构建出的真实二进制、`oz flow --help` 输出和 `oz flow contract --json`。
- 入口路径：执行 shell 契约测试，内部 `go build -o <tmp>/oz ./cmd/oz` 后调用真实 CLI。
- 关键断言：帮助文本必须包含 `oz flow run-acceptance --change <change-name> --json`；runner contract capabilities 必须包含 `run-acceptance`。
- 剩余风险：该场景只证明命令面可发现，不证明 required tests 已执行。

#### 场景：成功执行 active change 的 required tests 并检查 evidence

- 测试文件：`docs/changes/archive/2026-06-16-37-执行验收合同测试并汇总结果/tests/test_acceptance_run_success_contract.sh`
- 真实数据来源：测试脚本创建的临时 git 项目、真实 active change、真实 `acceptance.json`、真实 required test shell 脚本和本地 `test-results/` 运行产物。
- 入口路径：执行 shell 契约测试，内部在临时项目运行 `oz flow run-acceptance --change 1-验收执行样例 --json`。
- 关键断言：命令返回 0；JSON `valid=true` 且 `status=passed`；summary 显示 1 个测试通过；测试日志写入 `test-results/acceptance-run/1-验收执行样例/`；required evidence 文件存在并在 JSON 中标记为 `present`。
- 剩余风险：该场景只覆盖单测试成功路径，不覆盖失败汇总和 sealed run gate。

#### 场景：失败时仍汇总全部 required tests

- 测试文件：`docs/changes/archive/2026-06-16-37-执行验收合同测试并汇总结果/tests/test_acceptance_run_failure_contract.sh`
- 真实数据来源：测试脚本创建的临时 git 项目、两个真实 required test shell 脚本、一个通过脚本和一个失败脚本的运行日志。
- 入口路径：执行 shell 契约测试，内部运行 `oz flow run-acceptance --change 1-验收失败样例 --json`。
- 关键断言：命令返回非零；JSON `valid=false` 且 `status=failed`；summary 显示 2 个测试都执行过，其中 1 个 passed、1 个 failed；失败测试的退出码被记录；通过测试和失败测试的日志都存在。
- 剩余风险：该场景不要求并行执行，只要求失败不短路。

### 需求：验收合同 gate 集成

系统必须在 sealed run 的 execution 和 fix 阶段完成后复用同一 acceptance run 执行器，避免 review/QA 阶段才发现 required tests 未真实跑通。

#### 场景：execution/fix 后使用同一执行器阻断失败合同

- 测试文件：`docs/changes/archive/2026-06-16-37-执行验收合同测试并汇总结果/tests/test_acceptance_run_stage_gate_contract.sh`
- 真实数据来源：当前仓库 `internal/app` 生产代码、runner contract、状态模型和 Go 回归测试。
- 入口路径：执行 shell 契约测试，内部检查 acceptance run 边界源文件、状态字段、命令分发和 stage gate 回归测试。
- 关键断言：存在独立 acceptance run 源文件；存在 result DTO 和命令执行函数；execution/fix 后 gate 能保存最后结果路径和失败摘要；`go test ./internal/app` 中存在并通过 acceptance run 命令和 stage gate 相关回归。
- 剩余风险：该结构测试不替代真实外部 agent CLI 端到端运行；执行阶段仍需关注现有 workflow shell 合同。

### 需求：测试执行边界收敛

系统必须把 acceptance required tests 执行逻辑收敛到独立边界，避免 validation、QA artifact 校验和 runner contract 互相污染。

#### 场景：acceptance run 不混入 validation 和 QA artifact 逻辑

- 测试文件：`docs/changes/archive/2026-06-16-37-执行验收合同测试并汇总结果/tests/test_acceptance_run_stage_gate_contract.sh`
- 真实数据来源：当前仓库 `internal/app/validation.go`、`internal/app/qa.go`、`internal/app/runner_contract.go` 和新增 acceptance run 边界。
- 入口路径：执行 shell 契约测试，内部用源码结构断言和 Go 回归测试验证边界。
- 关键断言：`validation.go` 不直接遍历 `RequiredTests` 执行命令；`qa.go` 不直接执行 shell 命令；runner contract 只登记 capability；acceptance run 边界承载 required tests 执行和 evidence 检查。
- 剩余风险：结构断言允许实现细节调整，但必须保留独立边界和用户可见行为。

