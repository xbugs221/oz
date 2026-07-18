# 设计

## 1. Finding scope

在现有 `Finding` 合同上新增可选字段 `scope`：

- `acceptance_contract`：验收合同内目标未满足，必须阻断。
- `current_change`：当前 diff 或当前实现直接造成的问题，必须阻断。
- `introduced_regression`：当前变更引入的邻近功能回归，必须阻断。
- `out_of_scope_existing`：当前变更前已存在、且不影响本次 acceptance 的历史债务，不阻断当前提案。

缺省 `scope` 必须按 `current_change` 处理。这是兼容旧 artifact 的关键：旧格式仍保持原有严格行为，不会因为缺字段而突然放行。

## 2. 非阻断 finding

`review` 和 `qa` artifact 增加可选字段 `non_blocking_findings`。clean artifact 仍不得包含 `findings`，但可以包含 `non_blocking_findings`，且其中 finding 必须是 `out_of_scope_existing`。

这样 reviewer/QA 可以留下可复核证据，用户也能看到债务建议，但 workflow 不会把它误当作当前提案未完成。

## 3. Gate 判断

新增统一判断：

```text
finding scope
  acceptance_contract      -> hard block
  current_change           -> hard block
  introduced_regression    -> hard block
  out_of_scope_existing    -> non-blocking debt
  empty or unknown         -> invalid or legacy-hard-block
```

parallel review/QA gate 读取成员 findings 时，只把 hard-blocking scope 的 blocker/major finding 计入阻断。成员失败仍然保持硬阻断，因为失败表示该成员没有完成配置职责，无法安全判断范围。

## 4. QA acceptance 仍然严格

`acceptance_matrix` 继续只允许引用 `acceptance.json.required_tests` 和 `required_evidence` 中已有 id。历史债务不得通过新增 acceptance id 的方式混入当前提案验收；它只能进入 `non_blocking_findings`。

## 5. Prompt 更新

review 和 QA prompt 必须要求：

- 先判断 finding 是否属于当前 `acceptance.json`、当前 diff 或当前变更触达路径。
- 无关历史债务写入 `non_blocking_findings`，scope 使用 `out_of_scope_existing`。
- 对当前变更引入的问题不得降级为历史债务。
- 如建议修历史债务，应建议创建单独 oz change，而不是阻断当前 sealed run。

## 6. 兼容性

已创建但尚未运行的旧提案只依赖 `acceptance.json` 和 `docs/changes/archive/2026-06-10-12-收窄验收gate到提案范围/tests/`，没有运行期 review/QA artifact。执行本变更后，旧提案不需要迁移。

对旧运行 artifact：

- 缺少 `scope` 的 blocking finding 继续阻断。
- 缺少 `non_blocking_findings` 的 review/QA artifact 继续有效。
- `acceptance.json` 不新增必填字段。

## 风险

- 如果 scope 判断过宽，真实当前变更问题可能被误标为历史债务。因此 prompt 和 gate 都必须要求 out-of-scope finding 给出“为何不属于当前提案”的证据。
- 如果只改 prompt 不改 schema，模型仍可能输出无法被 gate 区分的 finding。因此本变更必须有机器字段和测试覆盖。
