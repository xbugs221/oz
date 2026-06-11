# 已完成执行跳过上下文 subagents

## 背景

`wo` 的 go-dag 工作流会在 execution 前运行 advisory parallel members，例如代码侦察和外部资料研究。它们对未开始执行的提案有价值，但当 `oz status` 已经表明提案 task 全部完成时，继续启动这些 subagents 只会重复侦察和消耗 agent 资源。

用户看到的典型状态是：规划和执行阶段已经完成，但执行前的代码侦察、外部资料仍然创建了会话并耗时完成。这说明调度顺序先跑了上下文 subagents，再由 execution 阶段判断任务是否已完成。

## 变更内容

- 在 execution 前增加轻量完成度判断，复用 `oz status <change> --json` 的 task 统计。
- 当 `tasks.total > 0 && tasks.done == tasks.total` 时，跳过 execution 前 advisory subagents、对应 fan-in artifact gate 和 execution 主 agent。
- 当 task 未完成时，保持现有代码侦察、外部资料、execution 主 agent 的完整路径。
- 保持 review、QA、archive 逻辑不变，避免把“执行任务已完成”误判为“提案已验收完成”。

## 非目标

- 不全局关闭 parallel 或 subagents。
- 不跳过 review/QA gate input subagents。
- 不通过代码 diff 自动判断实现是否正确。
- 不改变 `oz archive` 或已归档提案的语义。

