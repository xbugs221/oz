# 文件目的

本文件为执行阶段提供本提案的最短上下文，说明为什么要把 `acceptance.json.required_tests` 做成可执行、可汇总、可复核的工作流能力。

## 用户问题

当前仓库已经把 `acceptance.json` 校验做得很严格，但它主要证明合同形状正确、测试文件存在、evidence 有 producer。真正执行 `required_tests[].command`、汇总每个测试结果、检查 evidence 是否落盘、把结果接入 `oz flow` 后续 review/QA 的逻辑仍分散在人工操作、shell 规格脚本和阶段 prompt 中。

仓库里长期规格和归档提案下已经有大量 shell 合同脚本，维护者很难快速回答一个 active change 的核心验收合同是否已经真实跑过、哪些测试失败、哪些 evidence 缺失，以及失败结果应如何传给修复阶段。

## 交付目标

- 新增 `oz flow run-acceptance --change <change-name> --json`。
- 该命令读取 active change 的 `acceptance.json`，按顺序执行全部 `required_tests[].command`，不因第一个失败而跳过后续测试。
- 命令输出稳定 JSON，记录每个测试的状态、退出码、日志路径、耗时和 evidence 存在性。
- 命令把运行日志和汇总结果写入 `test-results/acceptance-run/<change-name>/`，这些文件保持为本地运行产物，不进入版本控制。
- `oz flow contract --json` 和帮助文本暴露该 runner 能力。
- sealed run 在 execution 和 fix 后复用同一执行器做验收测试 gate，失败时进入可修复的阻断状态，而不是让 review/QA 才发现合同测试没跑通。
- 执行阶段顺带收敛测试夹具和边界，避免继续把 acceptance 运行逻辑散落在 `validation.go`、`qa.go`、prompt 或 shell 脚本里。

## 非目标

- 不改变 `acceptance.json` 现有字段的向后兼容语义。
- 不把 `test-results/` 运行产物纳入版本控制。
- 不替代 QA artifact 审核；本变更只提供 required tests 的确定性执行结果，QA 仍需基于 acceptance matrix 做最终判断。
- 不强制所有历史归档提案测试立即迁移。
- 不引入新的第三方测试框架。

## 执行阶段默认上下文

执行器应先运行本提案 `acceptance.json.required_tests[].command`，确认它们因 `run-acceptance` 能力缺失而失败。实现时优先复用现有 `internal/acceptance`、`internal/app` 的 runner contract、validation artifact 和 QA acceptance matrix 逻辑，保持新增边界小而清晰。

