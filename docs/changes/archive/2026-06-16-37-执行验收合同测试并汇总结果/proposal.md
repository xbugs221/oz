# 文件目的

本文件说明本提案要把验收合同测试执行能力产品化，并解释它为什么是当前仓库的高价值重构路径。

## 背景

`oz` 的核心价值是让提案、规格、任务、测试和 QA evidence 形成可复查闭环。当前仓库已经完成了命令边界、配置解析、状态展示、Engine、subagent、状态持久化等多轮拆分，剩下的主要风险不再是单个 Go 文件过大，而是验收合同从“创建阶段写得很完整”到“执行阶段稳定跑通并留下统一证据”之间仍缺一个确定性入口。

现状里：

- `internal/acceptance` 能校验合同结构和 evidence producer。
- `oz validate` 能确认 active change 有测试文件、合同引用正确。
- `oz flow validate-qa` 能校验 QA artifact 是否覆盖 acceptance matrix。
- `validation.commands` 能跑仓库级门禁，但它不理解单个 change 的 `required_tests`。
- 大量 shell 规格脚本自己构造临时项目、写 acceptance、跑命令、采集日志，重复度高且输出形状不统一。

这导致维护者和自动化执行器缺少一个稳定问题答案：当前 change 的 required tests 是否已完整执行，失败在哪里，缺哪个 evidence，结果文件在哪里。

## 变更内容

新增 acceptance run 执行器，作为 `oz flow` 的确定性 runner 能力：

- `oz flow run-acceptance --change <change-name> --json`
- 读取 `docs/changes/<change-name>/acceptance.json`
- 执行全部 `required_tests[].command`
- 为每条测试保存独立日志
- 检查 `required_evidence[].path` 是否存在
- 输出机器可读 JSON
- 失败时返回非零退出码，但仍尽量报告所有测试和 evidence 状态
- `oz flow contract --json` 暴露 `run-acceptance` capability
- sealed run 在 execution/fix 后自动调用同一逻辑，把失败结果写入运行状态，供下一轮修复 prompt 读取

## 价值

- 把 acceptance 合同从静态文档推进为可直接执行的交付合同。
- 让执行、修复、review、QA 都围绕同一份 acceptance run result 说话。
- 减少 shell 规格脚本里重复的临时项目、日志、JSON 解析和 evidence 检查逻辑。
- 让后续重构测试夹具有稳定接口，而不是继续在每个提案脚本里复制执行模型。
- 保持 KISS：新增的是一个围绕现有合同的运行入口，不引入外部调度器或新测试框架。

