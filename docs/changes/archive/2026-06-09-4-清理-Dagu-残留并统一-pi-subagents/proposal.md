# 清理 Dagu 残留并统一 pi subagents

## 问题

默认执行路径已经是内嵌 `go-dag`，但用户仍能在 `wo graph`、`wo run --engine dagu` 和 `workflow.engine` 配置中看到或触发 Dagu/legacy 相关路径。这会让用户误以为当前工作流还有多个同级 engine，需要理解 Dagu 安装、Dagu YAML 和旧串行 engine 的差异。

同时，默认生成的 parallel subagent 配置仍写入 `tool: legacy-agent`，而当前仓库实际配置和用户预期已经切到 `pi`。新项目运行 `wo config` 后会得到过时默认值，导致规划、执行上下文与实际默认工具不一致。

当前 go-dag subagent 产物还缺少“正常退出后的即时格式校验和修正”边界。真实运行中 subagent 已经成功退出并写出 `SUBAGENT_OUTPUT`，但把 `evidence` 写成对象数组，读取端按 `[]string` 解码时才失败，导致整个 run 直接进入 failed。这个失败应被识别为单个 subagent artifact contract 失败，并 resume 同一 subagent 会话要求重写产物，而不是等到 fan-in 或主 workflow 阶段才暴露为底层 JSON 解析错误。

## 目标

- `workflow.engine` 对用户只保留 `go-dag`，缺省仍自动使用 `go-dag`。
- `wo graph` 只暴露当前支持的 `json` 和 `mermaid`，不再提供 Dagu YAML 导出。
- `wo run` 不再接受 `--engine dagu` 作为公开执行路径。
- 默认 `wo config` 生成的 planning/implementation parallel subagent 成员使用 `tool: pi`。
- 默认 `max_review_iterations` 从 `30` 降到 `5`，超过 5 轮修复应视为提案过粗或问题拆分不合理。
- `wo graph` 使用紧凑循环图表达 review/QA/fix 迭代，不再按每一轮重复绘制节点；用户可见 subagent 标签保留中文名称，不混入 `subagent`、`fan-in`、`planning_context` 等英文内部名。
- go-dag subagent 正常退出并写出 member artifact 后必须立刻触发 artifact schema gate；格式不正确时 resume 对应 subagent 会话要求修正，最多重试 3 次。

## 非目标

- 不改变 review、QA、fix、archive 的主状态机业务规则。
- 不改变 `json` / `mermaid` graph 的节点语义。
- 不要求删除与旧 run prompt snapshot 兼容相关的 `legacy` 命名。
- 不把新的 subagent artifact retry 机制落到 Dagu executor 上；Dagu 是待移除残留，新的 hook 必须属于 go-dag/subagent 共享执行边界。

## 验收

本提案通过两个真实 CLI 合同测试验收：

- `docs/changes/archive/2026-06-09-4-清理-Dagu-残留并统一-pi-subagents/tests/test_no_dagu_graph_engine_contract.sh`
- `docs/changes/archive/2026-06-09-4-清理-Dagu-残留并统一-pi-subagents/tests/test_default_subagent_pi_contract.sh`
- `docs/changes/archive/2026-06-09-4-清理-Dagu-残留并统一-pi-subagents/tests/test_compact_chinese_graph_and_iteration_limit.sh`
- `docs/changes/archive/2026-06-09-4-清理-Dagu-残留并统一-pi-subagents/tests/test_subagent_artifact_retry_contract.sh`

这些测试都从源码构建真实 `wo` 二进制，在临时 git 仓库中执行公开命令，避免依赖当前仓库的 `wo.yaml` 覆盖默认行为。
