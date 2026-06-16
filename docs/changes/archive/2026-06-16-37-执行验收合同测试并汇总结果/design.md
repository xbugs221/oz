# 文件目的

本文件记录验收合同执行器的设计决策、边界和风险。

## 设计原则

- 以 `acceptance.json.required_tests` 为唯一测试来源，不再从 prompt 或 task 文本推断要跑什么。
- 一次运行要执行全部 required tests，失败不短路，方便修复阶段拿到完整问题列表。
- 输出 JSON 必须稳定，供 runner、QA 和 shell 合同测试消费。
- 运行产物只写入 `test-results/`，不改变提案文件，不要求 evidence 进入 git。
- 新逻辑独立成边界文件，避免把合同测试执行塞进 `validation.go` 或 `qa.go`。

## 命令接口

新增：

```bash
oz flow run-acceptance --change <change-name> --json
```

初版只承诺 JSON 输出。非 JSON 输出可以给人类简要摘要，但本提案的硬合同只覆盖 JSON。

建议 JSON 形状：

```json
{
  "change": "1-demo",
  "valid": false,
  "status": "failed",
  "result_path": "test-results/acceptance-run/1-demo/result.json",
  "summary": {
    "total": 2,
    "passed": 1,
    "failed": 1,
    "evidence_total": 2,
    "evidence_present": 2,
    "evidence_missing": 0
  },
  "tests": [
    {
      "id": "contract-pass",
      "source": "change_contract",
      "path": "docs/changes/1-demo/tests/pass.sh",
      "command": "bash docs/changes/1-demo/tests/pass.sh",
      "status": "passed",
      "exit_code": 0,
      "log_path": "test-results/acceptance-run/1-demo/contract-pass.log",
      "duration_ms": 42
    }
  ],
  "evidence": [
    {
      "id": "runtime-log",
      "kind": "runtime_log",
      "path": "test-results/demo/runtime.log",
      "status": "present"
    }
  ],
  "error": ""
}
```

字段可以扩展，但不得删除本提案测试依赖的核心字段：`change`、`valid`、`status`、`summary`、`tests[].id`、`tests[].status`、`tests[].exit_code`、`tests[].log_path`、`evidence[].id`、`evidence[].status`。

## 执行模型

- 命令必须在 git repo root 下运行。
- 先复用 `ReadAcceptance` 或 `internal/acceptance.Read` 做严格合同解析。
- 对每个 `required_tests` 使用当前合同里的 `command` 执行，工作目录固定为 repo root。
- 每个测试独立捕获 stdout/stderr 到 `test-results/acceptance-run/<change>/<test-id>.log`。
- 每个测试记录开始时间、结束时间、退出码和状态。
- 全部测试结束后检查 `required_evidence[].path`。
- 汇总 JSON 同时写 stdout 和 `test-results/acceptance-run/<change>/result.json`。
- 任一测试失败、任一 evidence 缺失或 acceptance 合同读取失败时，命令返回非零退出码。

## 工作流集成

sealed run 中 execution 和每轮 fix 完成后，应先运行 acceptance run gate，再进入 review 或 QA：

- gate 通过：继续原有 stage decision。
- gate 失败但未达到 validation limit：将失败结果作为下一轮同阶段修复上下文。
- gate 失败且达到 limit：进入明确阻断状态，错误摘要指向 `result.json`。

可以复用现有 `StageValidationState` 和 validation artifact 存储，也可以新增 `AcceptanceRunState`。无论选择哪种实现，状态里必须能看到 acceptance run 的最后结果路径和失败摘要。

## 边界

- `internal/acceptance` 继续负责合同结构和 evidence producer 静态校验。
- `internal/app/acceptance_run.go` 或等价文件负责执行 required tests、检查 evidence 和生成结果。
- `internal/app/validation.go` 继续负责用户配置的仓库级 validation commands，不直接实现 acceptance required tests。
- `internal/app/qa.go` 继续负责 QA artifact 结构和 acceptance matrix 校验，不直接执行测试命令。
- `runner_contract.go` 只暴露 capability，不承载执行逻辑。

## 风险和取舍

- `required_tests[].command` 是 shell 字符串，已有合同就是这样记录命令。实现阶段应沿用现有语义，避免在本提案里重新设计 argv schema。
- 某些 required tests 可能较慢。本提案先要求正确性和完整汇总，不先引入并行执行。
- 历史提案的命令可能依赖环境。初版只保证 active change 的合同执行，不承诺批量跑所有归档提案。
- 如果 evidence 由外部 QA 手动采集，执行器只能报告缺失；不能伪造 evidence。

