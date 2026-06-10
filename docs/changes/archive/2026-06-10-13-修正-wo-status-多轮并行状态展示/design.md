# 设计

## 当前问题根因

### 规划行状态来源不完整

go-dag run 初始 `state.stage` 是 `execution`，没有真实 `planning` main_stage。规划上下文由 `planning_context_*` 和 `planning_context_fanin` 节点完成，但 `statusStageMarker` 只看 `state.Stages["planning"] == "completed"`，因此显示 `规划阶段 - - -`。

### human status 混入 fan-in summary

`buildHumanStatusView` 在每个 compact stage 后追加 `statusParallelGroupRows`。该函数读取 `parallel-*.json` 后把 group progress 和每个 member 的 raw `status` 输出为人类状态行。这个展示和极简状态视图冲突，也会因为 raw status 如 `LGTM_WITH_MINOR_CONCERNS` 或中文长句不在 `memberStatusSucceeded` 白名单里而误报 `failed`。

### 多轮阶段只用固定 parent stage 渲染子代理

compact stage spec 把审核阶段固定为 `review_1`。主行能通过 `matchingStatusStages` 聚合多轮，但 `statusParallelGroupRows` 和 `statusSubagentRows` 仍收到固定 `review_1`，因此子代理明细永远偏向第一轮。

### subagent node lookup 与主阶段节点 ID 碰撞

`statusSubagentNode` 对迭代 group 会生成候选 `review_3_3` 和 `before_review_3_3`，但也可能先命中其他同名或过宽候选。状态展示应只接受真实 subagent 节点前缀，不能把主阶段 `review_3` 当成第三个 review subagent。

## 方案

### human status 不展示 fan-in summary

从 human status/watch 输出中移除 `parallel_group` 和 `parallel_member` 行。保留短名 subagent 行，用 `DAGNodes` 和 `sessions` 表达真实 helper 执行状态。

JSON observability 可继续包含机器可读 artifact path，但 human fixed-column 输出不再渲染 fan-in summary。

### compact row 计算当前代表阶段

为每个 compact row 计算一个 `displayStage`：

- 固定阶段：`execution`、`archive`。
- 多轮阶段：优先使用当前 `state.Stage` 所在轮次；否则使用已到达阶段中的最后一轮。
- planning：若存在 `state.Stages["planning"] == "completed"`、`planning_context_fanin` success 或 `parallel-planning-context.json`，均视为完成。

`statusSubagentRows` 必须使用 `displayStage`，而不是固定 `review_1`。

### 多轮 marker 保留历史

对 `review_*`、`qa_*`、`fix_*`：

- 已完成轮次显示 `✓`。
- 当前失败轮次显示 `x`。
- 当前运行轮次显示 `→` 或 watch spinner。
- 不因为当前失败抹掉历史完成标记。

例如 `review_1`、`review_2` completed，`review_3` failed，应显示 `✓✓x`。

### 严格 subagent 节点候选

review/qa/fix 的 subagent node lookup 只接受 graph 生成的 helper 节点：

- review: `before_review_<iteration>_<memberIndex>`
- qa: `before_qa_<iteration>_<memberIndex>`
- implementation_context: `before_execution_<memberIndex>`
- planning_context: `planning_context_<memberIndex>`

不得把 `review_3`、`qa_2` 这类 main_stage node 当成 subagent。

### 规划 subagent 默认隐藏

对于 execution 起跑的 sealed run，planning_context 是执行前只读上下文。human status 的规划行只显示阶段完成状态，不展开规划 subagent 明细。这样避免用户误以为提案仍处于规划阶段。

## 风险

- 旧测试可能显式断言 `并行 implementation_context` 行存在，需要按新意图更新，不能简单删除覆盖。
- 如果有用户依赖 human status 中的 fan-in summary，需要改用 JSON observability 或 run artifact 文件。
- 历史 run 中缺失 `planning_context_fanin` 时仍可能无法推断规划完成，应保持保守显示。

