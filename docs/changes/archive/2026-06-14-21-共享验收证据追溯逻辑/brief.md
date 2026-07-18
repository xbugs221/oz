# 文件目的

本提案将验收 evidence producer 追溯逻辑沉到共享包，消除 `oz validate` 和 `wo` 预检的规则漂移。

## 用户问题

`cmd/oz/main.go` 和 `internal/app/acceptance_preflight.go` 都实现了 evidence producer 追溯。重复逻辑会导致 CLI 校验通过但工作流预检失败，或反向出现不一致。

## 交付目标

- 在 `internal/acceptance` 提供共享 producer 追溯 API。
- `oz validate` 和 `wo` acceptance preflight 都调用同一套规则。
- 保持原有错误语义可读，并补充共享包测试。

## 非目标

- 不改变 acceptance.json schema。
- 不扩大或放松 producer 追溯规则。
- 不引入外部 parser 或 shell 解析依赖。

## 验收入口

执行 `bash docs/changes/archive/2026-06-14-21-共享验收证据追溯逻辑/tests/shared-producer-contract_test.sh`。

## 执行阶段默认上下文

先读取 `internal/acceptance/acceptance.go`、`internal/app/acceptance_preflight.go`、`cmd/oz/main.go` 中的 acceptance 相关函数，再做最小抽取。
