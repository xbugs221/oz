# 提案：收敛工作流阶段状态和门禁流水线

## 问题

`oz flow` 的核心执行路径已经完成了多轮拆分，但仍保留两类高风险耦合：

- 阶段、运行状态、DAG node 状态和 validation 状态使用裸字符串流转，`done`、`completed`、`success`、`validation_failed` 这些词在不同文件中各自解释。
- `runLoop` 和 `nodeRunStage` 都在手写主阶段完成流程：检查 artifact、执行 acceptance preflight、执行 required tests、运行 validation、标记完成、推进下一阶段。

这会让未来新增 gate、调整状态展示或修复 DAG 行为时需要同时审查多处代码。任何一条路径漏改，都可能出现 loop 与 DAG 行为不一致。

## 目标

本次变更把“阶段/状态语义”和“主阶段完成门禁”收敛为可复用边界：

- 增加工作流阶段解析和状态规范化 helper，保持 JSON 持久化仍使用原字符串。
- 新增主阶段门禁流水线，统一 artifact gate、acceptance preflight、acceptance run、validation、stage completed、advance 的顺序。
- 让 `runLoop` 和 `nodeRunStage` 调用同一流水线，避免重复业务判断。
- 保持 `oz flow status`、`watch`、runner JSON、历史 `state.json` 兼容。

## 非目标

- 不改 `state.json` 字段名、阶段名或状态字符串。
- 不迁移历史 runtime state。
- 不改变 review/fix/QA/archive 的状态机语义。
- 不改变 batch 队列调度策略。

## 验收

创建阶段契约测试必须通过：

```bash
bash docs/changes/38-收敛工作流阶段状态和门禁流水线/tests/stage_gate_pipeline_contract_test.sh
```

执行阶段还必须运行：

```bash
go test ./internal/app
go test ./...
```
