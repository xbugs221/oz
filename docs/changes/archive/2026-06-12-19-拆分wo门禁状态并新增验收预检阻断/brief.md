<!-- 文件目的：概述 19 提案的用户问题、交付边界和执行阶段默认上下文。 -->

# 简报

## 用户问题

wo 目前把阶段产物门禁和确定性命令门禁都记录到 `state.validation`，导致
`execution` 的 `task.md` 未完成、测试命令失败、QA artifact schema 失败等不同问题在
状态里混成同一种 validation failure。复杂提案还会在 execution 后才逐轮暴露
`acceptance.json` 证据合同不可执行、live 环境依赖不清或 evidence 没有 producer 的问题，
浪费 review/QA/fix 轮次。

## 交付目标

- 将 stage artifact gate 的持久化状态从 command validation 中拆出。
- 保留现有 command validation 语义，只让它表示确定性命令执行结果。
- 在 execution artifact gate 通过后新增 acceptance preflight。
- 第一版 preflight 采用阻断策略：发现验收合同不可执行或 evidence 无法追溯 producer 时，
  run 进入专用阻断状态，让用户检查并修正验收合同。

## 非目标

- 不在本提案修复最后一轮 fix 缺少验证机会的问题。
- 不自动改写用户的 `acceptance.json`。
- 不引入 live WebUI、浏览器或外部服务执行。
- 不删除旧 run 对 `state.validation` 的兼容读取。

## 验收入口

创建阶段契约测试：

```bash
bash docs/changes/19-拆分wo门禁状态并新增验收预检阻断/tests/test_gate_state_and_acceptance_preflight_contract.sh
```

执行阶段默认上下文：

- `internal/app/state.go`
- `internal/app/stage_artifact_gate.go`
- `internal/app/validation.go`
- `internal/app/acceptance.go`
- `internal/acceptance/acceptance.go`
- `docs/specs/codex-workflow-cli/spec.md`
