# 设计

## 总体方案

本次变更把默认执行路径改为内嵌纯 Go DAG engine。执行库只负责在进程内调度依赖关系、并发执行、重试和 timeout；`wo` 自己继续维护业务图、状态文件、artifact 约束、status 输出和 graph 导出。

```text
WorkflowConfig
  -> BuildWorkflowSpec
  -> 构建 go-workflow DAG
  -> 节点执行时写 state.json / parallel-*.json / stage artifact
  -> wo status 渲染人类可读状态
  -> wo graph 导出 json 或 mermaid
```

## 关键决策

### 使用纯 Go DAG 库而不是 Dagu CLI

默认 engine 使用纯 Go 库，候选为 `github.com/Azure/go-workflow`。该库的职责与 `wo` 需求匹配：按依赖执行 DAG、ready step 并发运行、支持 retry/timeout/interceptor，并且不要求外部服务或二进制。

Dagu CLI 路径不再作为默认策略。现有 Dagu 相关代码可暂时保留为历史备份或迁移参考，但不得影响默认 `wo run`。

### `wo` 自己拥有图和可观测性

执行库不作为用户界面来源。`wo` 已有 `WorkflowSpec` 和 Mermaid/JSON graph exporter，本次继续强化这层：

- `wo graph --format mermaid` 展示完整 DAG 关系。
- `wo status` 展示当前 run 的 engine、总进度、并行成员和主阶段。
- 每个 DAG node 的状态写入 `state.json`，避免 status 依赖内存中的 goroutine 状态。

### 默认启用 parallel subagents

`defaultParallelConfig().Enabled` 改为 `true`，`wo config` 输出也保持一致。默认 DAG 在 execution 前运行 `planning_context` 与 `implementation_context`，在 review/QA 前运行对应 gate input 成员，再 fan-in 到既有 `parallel-*.json` artifact。

```text
planning_context members
  -> planning_context fan-in
  -> implementation_context members
  -> implementation_context fan-in
  -> execution
  -> review members
  -> review fan-in
  -> review
  -> qa members
  -> qa fan-in
  -> qa
  -> fix loop or archive
```

### 历史状态机降级

旧 Go 状态机保留为 hidden legacy path，方便紧急回退和对比调试。公开 help、默认配置和普通 `wo run` 不再把它描述为主路径。

## 状态模型

`state.json` 需要新增人类可观察字段，建议命名为：

```json
{
  "engine": "go-dag",
  "dag_nodes": {
    "planning_context/需求分析员": {
      "status": "success",
      "artifact": "...",
      "started_at": "...",
      "finished_at": "..."
    }
  }
}
```

字段名和内部结构可在执行阶段细化，但必须满足人类 status 和契约测试中的可见断言。

## 风险和处理

- **执行库不持久化历史**：由 `wo` 在 node interceptor 中写 `state.json`，把持久化职责留在本仓库。
- **parallel 默认开启增加耗时和资源占用**：通过 DAG 并发和配置项控制上限；status 必须清楚显示正在运行的成员。
- **旧 JSON contract 兼容**：runner JSON 不新增 parallel 结构字段，新增可观测信息只进入人类输出或内部 state。
- **图和执行不一致**：同一份 `WorkflowSpec` 同时用于 graph 导出和 DAG 构建，避免两套图定义漂移。

## 测试策略

创建阶段提供两个 shell 契约测试：

- 默认执行路径测试：用 fake agent 和 failing `dagu` 验证 `wo run` 默认不调用 Dagu，并要求状态中出现 `go-dag` 和 parallel 默认开启。
- 图和 status 测试：通过 `wo config`、`wo graph` 和 run-local state 验证默认图与人类 status 都能解释并行成员进度。
