# 设计：统一验收合同校验执行证据链

## 生命周期边界

建议在 `internal/acceptance` 中新增 lifecycle 层：

```text
acceptance lifecycle
  -> Parse/Validate contract
  -> Validate referenced files
  -> Trace evidence producer
  -> Summarize coverage
  -> Produce diagnostics
```

`internal/app` 可以继续拥有运行 required tests 的流程，但运行结果应携带 lifecycle diagnostics，避免 `run-acceptance` 只告诉用户 pass/fail 而缺少证据链上下文。

## 共享诊断

诊断对象建议包含：

```text
LifecycleDiagnostic
  code
  message
  test_id
  evidence_id
  path
  severity
```

`oz validate` 可以把错误诊断转成人类错误列表；`run-acceptance --json` 可以把诊断原样输出，方便 runner 和 QA 复核。

## 入口复用

```text
oz validate
  -> acceptance lifecycle validate files + producer

execution preflight
  -> acceptance lifecycle producer diagnostics

run-acceptance
  -> execute required tests
  -> acceptance lifecycle coverage/evidence diagnostics

validate-qa / stage artifact gate
  -> acceptance lifecycle required item set
```

## 兼容策略

- `acceptance.json` schema 不变。
- `AcceptanceRunResult` 只能新增字段，不能删除或重命名现有字段。
- 旧合同仍然通过 strict parser；新 diagnostics 是执行和展示层增强。

## 风险

- lifecycle 层如果过度抽象，会让简单校验变难读。缓解方式是先只收敛现有重复判断，不引入插件式规则系统。
- `run-acceptance` 输出新增字段可能影响严格消费者。缓解方式是只新增字段，并保留所有旧字段。
