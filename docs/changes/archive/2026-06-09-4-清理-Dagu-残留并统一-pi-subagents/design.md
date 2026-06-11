# 设计

## 当前结构

```text
wo CLI
  |
  +-- config.go
  |     +-- workflow.engine: go-dag / legacy / dagu
  |     +-- max_review_iterations: 30
  |     +-- defaultParallelConfig(): legacy-agent subagent tool
  |
  +-- graph.go
  |     +-- --format json
  |     +-- --format mermaid
  |     +-- --format dagu
  |     +-- review_i / qa_i / fix_i repeated per iteration
  |
  +-- app.go
        +-- default run -> go-dag
        +-- --engine dagu -> StartDaguJSON

  +-- dagu.go / go_dag.go
        +-- go-dag 当前复用 nodeRunSubagent/nodeFanin 等遗留 node helper
        +-- subagent artifact 解码失败时直接 fail run
```

问题不是默认执行会调用 Dagu，而是公开合同仍暴露 Dagu/legacy 分支，用户需要理解已经不该成为默认心智负担的路径。

另一个问题是 graph 展示把 `max_review_iterations` 展开成多轮节点。默认 30 轮会生成大量重复的 `review_N`、`qa_N`、`fix_N` 和 subagent fan-out/fan-in 节点；即使默认降到 5，逐轮复制仍会让图表达噪声大于业务含义。

第三个问题是 subagent member artifact 缺少完成后的 schema hook。subagent 正常退出后，代码会读取 `SUBAGENT_OUTPUT`，但如果 JSON 字段类型不匹配，例如 `evidence` 写成对象数组而不是字符串数组，当前行为会把底层 unmarshal 错误直接提升为 run failed。这个错误属于单个 subagent 产物格式错误，应该反馈给同一 subagent 会话修正，而不是让整个 workflow 立即失败。

## 方案

### 收窄 engine 合同

配置读取只接受空值或 `go-dag`。空值仍归一化为 `go-dag`，保证没有配置的仓库继续可用。

如果 `wo.yaml` 写了 `legacy` 或 `dagu`，命令必须在读取配置阶段失败，并提示 `workflow.engine` 只支持 `go-dag`。这样用户不会在后续执行时才遇到 Dagu CLI 或旧串行路径的分支错误。

### 去掉 Dagu graph/exporter

`wo graph` 的公开格式收敛为：

```text
wo graph --change <change-name> --format json
wo graph --change <change-name> --format mermaid
```

`--format dagu` 不再是可选项，错误提示也不得继续把 `dagu` 列为可选格式。保留 `WorkflowSpec` 作为 graph 的内部中间结构，避免影响 `go-dag` 调度和 Mermaid 输出。

### 去掉公开 Dagu run 路径

`wo run --engine dagu --json` 不再进入 Dagu CLI 检查或 Dagu YAML 写入。实现上可以删除 `--engine` 参数，或仅接受 `--engine go-dag` 作为无副作用兼容输入；但不得继续把 `dagu` 当作可执行 engine。

### 抽离 go-dag subagent 执行边界

当前 `go_dag.go` 仍会调用 `dagu.go` 中的 `nodeRunSubagent`、`nodeFanin`、`nodeRunStage` 等 helper。清理 Dagu 时不能把新的 subagent 行为继续塞进 Dagu executor 文件。实现应新增或迁移到 go-dag/subagent 语义明确的模块，例如：

```text
internal/app/subagent.go
  +-- runSubagentMember(...)
  +-- validateSubagentMemberArtifact(...)
  +-- retrySubagentArtifactRepair(...)
```

`go_dag.go` 调用新的 subagent 执行边界；Dagu 相关入口在本提案中删除或隔离，不再承载新业务规则。

### subagent artifact schema hook

每个 subagent member 的执行必须形成如下闭环：

```text
启动 subagent
  -> subagent 正常退出并返回 session id
  -> 立即读取 SUBAGENT_OUTPUT
  -> 执行 member artifact schema gate
  -> 通过：补齐 Purpose/Required 并写回规范化 artifact
  -> 失败：resume 同一 session，要求只重写 SUBAGENT_OUTPUT
  -> 最多 3 次
  -> 仍失败：run failed，错误包含成员名、字段、期望类型和 artifact 路径
```

schema gate 必须明确校验：

- `name`、`purpose`、`status`、`summary` 为 string
- `evidence` 为 `string[]`
- `findings` 为对象数组
- `findings[].title`、`findings[].severity`、`findings[].evidence`、`findings[].recommendation` 为 string
- 不允许对象型 `evidence`、嵌套数组或 markdown 包裹 JSON

重试 prompt 必须带上确定性失败原因，例如 `evidence 必须是字符串数组，当前第 1 项是 object`，并要求 agent 只重写 `SUBAGENT_OUTPUT`。如果第一次 run 返回了 session id，第二、三次修正必须使用相同 session id resume 对应 subagent；不得新开无上下文会话修格式。

这个 hook 的目标是把 LLM 产物格式错误限制在 subagent 局部修正，不改变 review/QA/fix/archive 主状态机规则，也不放宽最终 artifact 合同。

### 默认 subagent tool 改为 pi

同时更新：

- `defaultParallelConfig()` 中 planning/implementation 成员的 `Tool`
- `DefaultWorkflowConfigYAML` 中 planning/implementation 成员的 `tool`
- 默认配置相关测试期望

仓库现有 `wo.yaml` 已经使用 `tool: pi`，本变更让新项目生成配置与当前使用约定一致。

### 默认迭代数改为 5

`defaultMaxReviewIterations` 和默认 `wo.yaml` 模板都改为 `5`。设计理由是：review/QA/fix 超过 5 轮通常说明提案范围、验收合同或问题拆分有缺陷，继续扩大自动循环预算只会掩盖需求不清。

### graph 改为紧凑中文状态图

`BuildWorkflowSpec` 或 Mermaid 导出层应区分执行 DAG 和用户展示 graph。`go-dag` 执行仍可使用有限节点调度，但 `wo graph --format mermaid` 面向人类理解，应表达状态机：

```text
规划上下文 -> 执行 -> 审核 -> 测试 -> 归档
                    |      |
                    +-- 修复 <+
```

parallel subagent 在图中使用中文可见标签：

```text
需求分析员
代码库侦察员
外部资料研究员
规划上下文汇总
执行上下文汇总
```

节点 ID 可继续使用 ASCII，便于 Mermaid 和 JSON 稳定解析；用户可见节点名称、edge label 和摘要不得混入 `subagent`、`fan-in`、`planning_context`、`implementation_context` 这类内部英文名。

## 风险

- 老配置中显式写了 `engine: legacy` 或 `engine: dagu` 的仓库会在启动时失败。失败是目标行为，需要给出清楚迁移提示。
- 如果有外部脚本依赖 `wo graph --format dagu`，会被显式破坏。该路径属于本次清理目标，不保留兼容。
- `go-dag` 仍复用现有 workflow node helper 时，代码中可能保留 `node` 概念；验收关注用户可见 Dagu contract 是否清掉，不要求一次性重命名所有内部 helper。
- Mermaid 图改为紧凑视图后，不再逐个展示每轮节点；需要通过文案或 label 表达“最多 5 轮”这个业务约束，避免用户误解为只有 1 轮。
- subagent artifact retry 可能让单个成员多跑两次；实现必须限制为最多 3 次，且每次后仍执行只读边界检查，避免“修格式”时修改源码或提案文档。
- 如果 agent CLI 正常退出但没有返回 session id，仍可重试格式修正，但错误和状态必须说明无法 resume 原会话；默认路径应优先覆盖 pi/codex/legacy-agent 的已有 session id 提取能力。
