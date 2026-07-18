# 简报：收敛工作流阶段状态和门禁流水线

本提案解决 `oz flow` 核心工作流中阶段、状态和门禁执行逻辑分散的问题。当前 loop 运行路径和 Go DAG node 路径都在分别处理 artifact gate、acceptance preflight、acceptance run、validation、stage completed 和 advance，阶段/状态又大量使用裸字符串。行为已经有测试保护，但后续改动容易出现两条执行路径不一致。

交付目标是建立一套轻量的阶段/状态语义边界，并把主阶段完成流程收敛成单一门禁流水线。执行阶段默认先阅读本目录的 `acceptance.json`、`docs/changes/archive/2026-06-17-38-收敛工作流阶段状态和门禁流水线/tests/stage_gate_pipeline_contract_test.sh`、`spec.md` 和现有 `internal/app/{engine_run.go,node.go,stage_decision.go,state_model.go,status_view_model.go}`。

非目标：不迁移历史 `state.json`，不更改公开阶段名，不改变 retry 次数、DAG 图结构、status/watch/runner JSON 的既有字段。

验收入口：`bash docs/changes/archive/2026-06-17-38-收敛工作流阶段状态和门禁流水线/tests/stage_gate_pipeline_contract_test.sh`。该测试会生成 `test-results/38-stage-gate-pipeline/contract.log`，作为 QA 可复核运行证据。
