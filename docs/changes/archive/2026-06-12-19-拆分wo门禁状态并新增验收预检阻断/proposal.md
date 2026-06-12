<!-- 文件目的：说明 19 提案要做什么以及为什么要做。 -->

# 提案：拆分 wo 门禁状态并新增验收预检阻断

## 背景

wo 的主阶段有两类现有门禁：

- stage artifact gate：检查当前阶段是否写出合法产物，例如 `task.md` 全勾、
  `review-N.json` 合法、`qa-N.json` 覆盖 acceptance matrix。
- validation gate：运行配置里的确定性命令，例如 `go test ./...` 或项目自定义脚本。

当前实现把 artifact gate 的失败也写进 `state.validation`，只用 `kind=artifact`
区分。这让状态语义变得含混：用户看到 validation failed 时无法直接判断是命令失败、
阶段产物缺失，还是验收合同本身不可执行。

同时，wo 只有到 review/QA 阶段才会逐项发现 `acceptance.json` 中的证据合同问题。
当 required evidence 依赖 live 环境、没有 producer 或只是静态说明文件时，工作流会
消耗多轮 review/QA/fix 才暴露不可收敛的根因。

## 变更内容

本提案引入三个清晰职责：

- `artifact_gates`：只记录阶段产物门禁。
- `validation`：继续兼容当前字段，但新 run 中只记录命令 validation gate。
- `acceptance_preflight`：execution artifact gate 通过后、进入 review 前执行的验收合同预检。

第一版 acceptance preflight 不修复合同，只阻断：

```text
execution
  -> artifact_gates.execution passed
  -> acceptance_preflight
       clean  -> review_1 / archive
       failed -> blocked_acceptance_contract
```

preflight 只做结构和可执行性检查，不跑完整业务验收。它应识别：

- required evidence 没有可追溯 producer。
- live / screenshot / network / console 证据没有声明可复核入口或环境条件。
- evidence 目录可能由多个 producer 覆盖但没有最终完整性检查。
- acceptance matrix 的 ID 映射无法从 `required_tests` / `required_evidence` 建立闭环。

## 为什么这样做

将 artifact gate 和 validation gate 拆开后，状态机可以明确给出下一步动作：

- 阶段产物不合格：同阶段重试，让 agent 补写当前 artifact。
- 命令 validation 失败：同阶段重试，让 agent 修代码或测试。
- acceptance preflight 失败：停止自动实现，让用户检查验收合同。

这能避免“合同本身不可执行”被误判成实现 bug，也避免复杂提案在 review/QA 里逐轮消耗
修正额度才发现证据设计问题。
