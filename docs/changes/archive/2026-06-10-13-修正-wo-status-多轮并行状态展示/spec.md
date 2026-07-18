# 规格

## 验收矩阵

| 需求 | 场景 | required_tests | required_evidence |
| --- | --- | --- | --- |
| 极简 human status 不泄漏 parallel fan-in | 多轮 run 不显示并行 summary/raw member status | `status-multiround-parallel-display-contract` | `status-multiround-parallel-display-log` |
| 规划阶段完成态正确 | execution 起跑且 planning_context fan-in 成功时规划行显示完成且隐藏规划 subagent | `status-multiround-parallel-display-contract` | `status-multiround-parallel-display-log` |
| 多轮阶段 marker 保留历史 | 第三轮 review 失败时审核行显示 `✓✓x` | `status-multiround-parallel-display-contract` | `status-multiround-parallel-display-log` |
| 子代理明细绑定当前轮次 | review_3 失败时显示第三轮 review subagent session 且不把主阶段 node 误判为 subagent | `status-multiround-parallel-display-contract` | `status-multiround-parallel-display-log` |

### 需求：极简 human status 不泄漏 parallel fan-in

系统必须让 human `wo status` 和 `wo watch` 保持固定列极简输出。parallel fan-in artifact 是内部聚合产物，不得以 `- 并行 <group>` 或 fan-in member raw status 的形式显示给用户。

#### 场景：多轮 run 不显示并行 summary/raw member status

- **对应测试**：`docs/changes/archive/2026-06-10-13-修正-wo-status-多轮并行状态展示/tests/test_status_multiround_parallel_display_contract.sh`
- **真实数据来源**：测试在临时 git 仓库中创建真实 `State`、`StageTimings`、`DAGNodes`、`sessions`、`parallel-implementation-context.json`、`parallel-review-1/2/3.json` 和 member artifact 路径，复现一个 execution 完成、两轮 fix 完成、第三轮 review 失败的 run。
- **入口路径**：`buildHumanStatusView`、`compactStatusLines`、`statusSubagentRows`、`statusStageMarker`。
- **关键断言**：输出不包含 `- 并行`、`implementation_context`、`parallel-review`、`LGTM_WITH_MINOR_CONCERNS`、`completed - -`；仍保留短名 subagent 行。
- **剩余风险**：该测试不验证外部终端 TTY 刷新，只验证 status/watch 共用的 compact view 内容。

### 需求：规划阶段完成态正确

系统必须把 execution 起跑的 sealed run 中已完成的 `planning_context` fan-in 视为规划准备完成。human status 不得因为没有真实 `planning` main_stage 而显示 `规划阶段 - - -`。

#### 场景：execution 起跑且 planning_context fan-in 成功时规划行显示完成且隐藏规划 subagent

- **对应测试**：`docs/changes/archive/2026-06-10-13-修正-wo-status-多轮并行状态展示/tests/test_status_multiround_parallel_display_contract.sh`
- **真实数据来源**：同一 fixture 写入 `planning_context_1/2/3` 和 `planning_context_fanin` 成功 DAG node，并写入 `parallel-planning-context.json`。
- **入口路径**：`statusStageMarker`、`statusSubagentRows`、`statusGroupsForStage` 或新增 planning 状态推断 helper。
- **关键断言**：输出包含 `规划阶段 - ✓ -`；输出不包含 `需求分析`、`review1-target` 这类已过时 helper 明细。
- **剩余风险**：历史 run 如果没有 planning fan-in artifact 或 DAG node，仍可保守显示未完成。

### 需求：多轮阶段 marker 保留历史

系统必须在 compact 阶段行展示已发生轮次的历史状态。当前轮次失败不能抹掉前序成功轮次。

#### 场景：第三轮 review 失败时审核行显示 `✓✓x`

- **对应测试**：`docs/changes/archive/2026-06-10-13-修正-wo-status-多轮并行状态展示/tests/test_status_multiround_parallel_display_contract.sh`
- **真实数据来源**：同一 fixture 设置 `review_1` 和 `review_2` completed，`review_3` failed，且 `fix_1`、`fix_2` completed。
- **入口路径**：`matchingStatusStages`、`statusStageMarker`、`statusStageDuration`。
- **关键断言**：审核阶段行包含 `审核阶段 reviewer-session ✓✓x`；修正阶段行仍包含 `修正阶段 fixer-session ✓✓`。
- **剩余风险**：本场景覆盖 review/fix，多轮 QA 使用同一 marker 逻辑，应由执行阶段补充邻近单元测试或复用 helper 覆盖。

### 需求：子代理明细绑定当前轮次

系统必须让 review/qa 子代理明细跟随当前 compact stage 代表轮次。`review_3` 失败时，审核阶段下的子代理应显示第三轮 subagent session 和 DAG node 状态。

#### 场景：review_3 失败时显示第三轮 review subagent session 且不把主阶段 node 误判为 subagent

- **对应测试**：`docs/changes/archive/2026-06-10-13-修正-wo-status-多轮并行状态展示/tests/test_status_multiround_parallel_display_contract.sh`
- **真实数据来源**：同一 fixture 写入 `before_review_1_*`、`before_review_2_*`、`before_review_3_*` 三轮 helper DAG node；第三轮 helper 全部 success，但主阶段 `review_3` node failed。
- **入口路径**：`statusSubagentRows`、`statusSubagentNode`、`statusSubagentSessionID`、`statusGroupIteration`。
- **关键断言**：输出包含第三轮 session `review3-target`、`review3-quality`、`review3-test`、`review3-risk`、`review3-context`；输出不包含第一轮 session `review1-target`；`测试有效 review3-test ✓` 不能因为主阶段 `review_3` failed 被标成 `x`。
- **剩余风险**：该测试不要求展示所有历史轮次 subagent，仅要求当前代表轮次正确。

