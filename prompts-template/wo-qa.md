读取:

- `{{.StatePath}}`
- `{{.AcceptancePath}}`
- `{{.ReviewPath}}`
- 对应 oz change：`{{.ChangePath}}/`
{{if .HasPlanningContext}}
- 规划并行上下文 artifact：`{{.PlanningContextPath}}`
{{end}}
{{if .HasParallelContext}}
- 执行前并行上下文 artifact：`{{.ParallelContextPath}}`
{{end}}
{{if .HasParallelQA}}
- 并行 QA 输入 artifact：`{{.ParallelQAPath}}`
{{end}}

验收：

- 以真实用户路径复测当前实现，优先运行项目的端到端测试、启动应用并采集截图/trace/控制台证据
- 必须逐项执行或核对 `{{.AcceptancePath}}` 中的 `required_tests` 和 `required_evidence`
- Web app 必须检查交互状态变化、刷新后状态、加载/空/错误态、控制台错误、网络失败和桌面/移动端基本观感
- 不修改源码，不修改 `{{.AcceptancePath}}`；如发现问题，只写入结构化 QA artifact
{{if .HasParallelQA}}
- 如 `{{.ParallelQAPath}}` 已存在，必须把其中真实路径 QA 结果汇总进最终 `{{.QAPath}}` 的 `evidence` 和 `acceptance_matrix`。
- 如尚不存在该 artifact，先按 `workflow_config.parallel.groups.qa` 的职责并行执行 CLI/API、浏览器路径、证据采集和回归场景验证，并写入 `{{.ParallelQAPath}}`。
- tool/subagent 只作为提示词角色线索，不作为 CLI 参数；当前主 agent 负责按成员 `name/purpose/stage/tool/subagent` 组织产物。
- 任一 gate_input 成员缺少 required evidence 或报告 blocker/major finding 时，最终 QA `decision` 不得为 `clean`。
{{end}}

写入：

```text
{{.QAPath}}
```

严格 JSON 要求：

- 只输出一个 JSON 对象，不得输出 Markdown、注释、代码块或任何 JSON 以外文本。
- `decision` 只允许 `clean` 或 `needs_fix`。
- `clean` 时 `findings` 为空，`evidence` 必须包含可复核的测试、截图、trace、控制台或运行时证据。
- `clean` 时 `acceptance_matrix` 必须覆盖 `{{.AcceptancePath}}` 中每个 `required_tests[].id` 和 `required_evidence[].id`，且每项 `status` 必须为 `passed`。
- `acceptance_matrix[].id` 只能逐字使用 `{{.AcceptancePath}}` 中已经定义的 id，不得新增、概括、合并或改写 id。
- `needs_fix` 时 `findings` 至少一项，finding 必须包含 `title`、`severity`、`evidence`、`recommendation`。
- `severity` 仅允许 `blocker`、`major`、`minor`（内部会归一化 `high/medium/low/nit/critical`）。
- 如果 prompt 追加了 `Stage artifact gate failed`，说明上一次 QA artifact 没通过 schema 或 acceptance 合同门禁；必须先按错误摘要重写 `{{.QAPath}}`，不要修改源码或改 acceptance 合同。
{{if .HasRoleSession}}
- 复用当前角色会话：`{{.RoleSessionKey}}`，本轮不重复 JSON 示例。
{{end}}

{{if .IsFirstRoleTurn}}
clean 示例：

```json
{
  "summary": "核心业务路径已通过 QA",
  "decision": "clean",
  "findings": [],
  "acceptance_matrix": [
    {
      "id": "contract-order-filter",
      "status": "passed",
      "artifact": "docs/changes/12-订单筛选/tests/order-filter.acceptance.test.ts",
      "evidence": "pnpm exec tsx --test docs/changes/12-订单筛选/tests/order-filter.acceptance.test.ts passed"
    },
    {
      "id": "screenshot-filter-after-refresh",
      "status": "passed",
      "artifact": "test-results/order-filter/after-refresh.png",
      "evidence": "screenshot artifact shows the selected filter and matching list after page.reload()"
    }
  ],
  "evidence": [
    "pnpm exec playwright test passed",
    "screenshot artifact: test-results/orders-checkout-desktop.png",
    "console/network check: no browser console errors or failed API calls"
  ]
}
```

needs_fix 示例：

```json
{
  "summary": "刷新后订单筛选状态丢失",
  "decision": "needs_fix",
  "findings": [
    {
      "title": "刷新后筛选状态丢失",
      "severity": "major",
      "evidence": "Playwright trace test-results/orders-filter.zip shows filter reset after page.reload()",
      "recommendation": "将筛选状态持久化到 URL query 或等价可恢复状态，并补刷新后断言"
    }
  ],
  "evidence": [
    "pnpm exec playwright test tests/e2e/orders.spec.ts failed",
    "trace artifact: test-results/orders-filter.zip"
  ]
}
```
{{end}}
