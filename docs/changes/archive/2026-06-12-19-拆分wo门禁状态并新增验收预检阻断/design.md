<!-- 文件目的：记录 19 提案的关键技术决策、取舍和风险。 -->

# 设计

## 状态拆分

新增状态字段建议：

```go
ArtifactGates       map[string]StageValidationState `json:"artifact_gates,omitempty"`
AcceptancePreflight AcceptancePreflightState        `json:"acceptance_preflight,omitempty"`
```

`StageValidationState` 可暂时复用现有字段，避免第一版引入过多类型迁移。关键要求是新 run
的 artifact gate failure 不再写入 `State.Validation`。`State.Validation` 继续保留，用于
兼容旧 run 和 command validation gate。

`AcceptancePreflightState` 至少需要：

- `status`
- `last_error`
- 可复核 artifact 路径或 findings 摘要

执行阶段可以选择复用 `StageValidationState`，但 JSON 字段必须独立，不得把 preflight
失败写成 `validation.execution`。

## acceptance preflight 最小规则

第一版不扩展 `acceptance.json` schema。preflight 使用现有字段做保守判断：

- `required_tests` 必须存在且命令非空，沿用现有 acceptance schema。
- 每个 `required_evidence` 必须能在同一 acceptance 合同中追溯到生产者。
- 生产者可由 `required_tests[].command`、`path`、`purpose`、`assertions` 或 coverage
  中的测试关联提供；如果只能从 coverage 看出“需要该 evidence”，但没有任何测试或命令
  说明如何生成该文件，则判定失败。
- `screenshot`、`trace`、`network`、`console` 这类运行证据必须有明确可复核入口，不得只靠
  手写说明文件。

这套规则故意保守。它宁愿阻断用户检查合同，也不让 wo 在不清楚证据来源时继续自动修实现。

## 调度位置

preflight 放在 execution artifact gate 通过之后：

```text
execution agent 返回
  -> checkStageArtifactGate(execution)
  -> runAcceptancePreflight
  -> validateStage(command validation, 如果配置了)
  -> advance 到 review/archive
```

如果实现阶段发现 command validation 必须早于 preflight，可以调整顺序，但必须保证：

- `task.md` 未全勾时不跑 preflight。
- preflight failed 时不进入 review/QA/fix/archive。
- preflight failed 不消耗 review/fix 轮次。

## 兼容策略

- 旧 run 中已有 `validation[stage].kind == "artifact"` 的记录继续显示。
- 新 run 不再向 `validation` 写 artifact gate failure。
- status 输出可同时展示旧 `validation` 和新 `artifact_gates`。

## 风险

- producer 追溯规则第一版只能做启发式检查，不能完全证明 evidence 一定会被生成。
- 如果现有 acceptance 合同大量只写 evidence path，不写生产方式，执行本提案后会更早阻断。
  这是预期行为，因为这些合同本身不可稳定收敛。
- 后续如果需要更强约束，可以单独提案给 `required_evidence` 增加显式 `producer` 字段。
