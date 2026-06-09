# 统一 wo status/watch 极简状态输出

## 背景

`wo status` 和 `wo watch` 都是用户观察工作流进度的关键入口，但当前两者的输出结构不同：`status` 展示 engine、并行 group 摘要、成员明细和耗时汇总，`watch` 又单独组装一套标题和阶段列表。随着 go-dag、parallel subagents 和耗时统计陆续加入，两套渲染逻辑已经出现明显漂移。

用户最终确认的目标是最大程度清晰简洁：

```text
| w1
规划阶段 planner-session ✓ 2.00
执行阶段 writer-session → 6.50
  代码侦察 subagent-session-1 ✓ 1.10
  外部资料 subagent-session-2 ✓ 0.80
审核阶段 reviewer-session - -
测试阶段 - - -
归档阶段 - - -
```

## 变更目标

- `wo status` 和 `wo watch` 使用同一份状态视图；`watch` 只改变第一行 indicator。
- 单 workflow 人类输出第一行只显示 indicator 和短 workflow 编号，不显示 `工作流`、状态单词、change name、engine、并行汇总或总耗时行。
- 主阶段和子代理都使用固定四列：`名称 会话id 进度标记 耗时分钟`。
- 子代理行缩进两格，并使用短名称，例如 `代码侦察`、`外部资料`、`目标核对`、`风险检查`。
- batch 输出只是外层队列加多个 workflow 视图，不再另起一套 batch 内 run 样式。
- `wo status --run-id <run-id> --json` 保留既有 runner 顶层字段，并新增结构化 observability 信息，给出主阶段、子代理和固定产物路径。

## 非目标

- 不引入颜色、表格、边框、emoji 或终端宽度自适应布局。
- 不改变工作流执行顺序、DAG 调度或 artifact gate 语义。
- 不删除既有 runner JSON 顶层字段。
- 不要求 human 输出展示所有 artifact 路径；路径只进入 JSON。
- 不在本次变更中修复 `cmd/wo` 入口缺失问题；本提案测试直接覆盖 `internal/app` 中的 `wo status` 命令解析和渲染路径。

## 验收重点

- `wo status -w1` 输出极简 workflow 视图，第一行为 `→ w1`。
- `wo watch -w1` 使用同一套正文，第一行为 spinner 和 `w1`。
- 子代理不再显示并行 group 汇总行，而是作为主阶段下的缩进固定列明细。
- batch 输出外层只显示 `indicator bN current/total` 和 change 列表，已创建 run 内嵌同一套 workflow 视图。
- JSON 新增 `observability.rows` 和 `observability.artifacts`，下游工具能从固定字段定位 stage artifact、subagent artifact、fan-in artifact、run state 和 change 文档。
