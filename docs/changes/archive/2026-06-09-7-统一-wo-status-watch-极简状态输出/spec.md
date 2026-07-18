# 规格

## 验收矩阵

| 需求 | 场景 | required_tests | required_evidence |
| --- | --- | --- | --- |
| 极简 workflow human 输出 | 单 workflow status/watch 使用同一套固定列视图 | `status-watch-compact-output-contract` | `status-watch-compact-output-log` |
| batch 组合 workflow 视图 | batch 外层只包装多个极简 workflow 视图 | `status-watch-compact-output-contract` | `status-watch-compact-output-log` |
| JSON 可观测产物路径 | JSON rows 给出阶段和子代理固定产物路径 | `status-json-observability-artifacts-contract` | `status-json-observability-artifacts-log` |
| JSON 兼容旧 runner 字段 | 新增 observability 时保留既有 runner 顶层字段 | `status-json-observability-artifacts-contract`, `go-test-internal-app-regression` | `status-json-observability-artifacts-log` |

### 需求：极简 workflow human 输出

系统必须让单 workflow 的 `wo status` 和 `wo watch` 使用同一套极简固定列视图。输出不得再显示 `工作流` 标题、状态英文单词、change name、engine 行、并行 group 汇总行或总耗时行。

#### 场景：单 workflow status/watch 使用同一套固定列视图

- **对应测试**：`docs/changes/archive/2026-06-09-7-统一-wo-status-watch-极简状态输出/tests/test_status_watch_compact_output_contract.sh`
- **真实数据来源**：测试在临时 git 仓库中创建真实 run state、真实 `stage_timings`、真实 `dag_nodes`、真实 subagent session 记录和真实 `parallel-members/...json` / `parallel-implementation-context.json` artifact。
- **入口路径**：调用 `internal/app.Run([]string{"status", "-w1"}, ...)` 覆盖 `wo status -w1` 命令解析和状态读取；调用 `watchStatusLines` 覆盖 `wo watch -w1` 的刷新帧渲染。
- **关键断言**：`status` 第一行为 `→ w1`；`watch` 第一行为 `| w1`；主阶段行固定为 `规划阶段 planner-session ✓ 2.00`、`执行阶段 writer-session → 6.50` 等四列；子代理行缩进两格并显示 `代码侦察`、`外部资料` 短名；输出不包含 `工作流`、`引擎`、`并行`、`耗时`、`implementation_context` 或完整子代理长名。
- **剩余风险**：该测试不通过真实终端 TTY 验证原地刷新，只验证每一帧的内容合同。

### 需求：batch 组合 workflow 视图

系统必须让 batch 输出只作为多个 workflow 视图的外层组合。batch 标题行显示 indicator、batch 短编号和队列进度；每个已创建 run 内嵌同一套单 workflow 极简视图。

#### 场景：batch 外层只包装多个极简 workflow 视图

- **对应测试**：`docs/changes/archive/2026-06-09-7-统一-wo-status-watch-极简状态输出/tests/test_status_watch_compact_output_contract.sh`
- **真实数据来源**：同一测试创建真实 batch state，包含一个已创建 running run 和一个尚未开始的 change。
- **入口路径**：调用 `watchStatusLines(repo, "batch", StatusRef{Alias:"b1", ...}, "|")` 覆盖 `wo watch -b1` 的 batch 帧渲染。
- **关键断言**：batch 第一行为 `| b1 1/2`；已创建 run 在 change 名下缩进展示 `→ w1` 和固定列阶段/子代理行；未开始 change 只显示 change 名，不伪造 workflow 行。
- **剩余风险**：该场景不验证 failed batch 的恢复提示；失败恢复提示沿用既有失败摘要测试覆盖。

### 需求：JSON 可观测产物路径

系统必须在 `wo status --run-id <run-id> --json` 中新增 `observability` 字段，下游工具可以从固定字段定位每个主阶段、子代理和 change/run 的产物。

#### 场景：JSON rows 给出阶段和子代理固定产物路径

- **对应测试**：`docs/changes/archive/2026-06-09-7-统一-wo-status-watch-极简状态输出/tests/test_status_json_observability_artifacts_contract.sh`
- **真实数据来源**：测试在临时 git 仓库中创建真实 change 文档、真实 run state、真实 DAG node artifact 路径和真实 subagent artifact 文件。
- **入口路径**：调用 `internal/app.Run([]string{"status", "--run-id", runID, "--json"}, ...)` 覆盖 `wo status --run-id <run-id> --json` 路径。
- **关键断言**：JSON 包含 `observability.engine`、`observability.rows` 和 `observability.artifacts`；执行阶段 row 的 `artifacts.stage_artifact` 指向 `docs/changes/<change>/task.md`；审核、测试、归档阶段即使尚未开始也给出 `review-1.json`、`qa-1.json`、`delivery-summary.md` 预期路径；子代理 row 给出 `member_artifact` 和 `group_artifact` 绝对路径。
- **剩余风险**：测试只覆盖 implementation context 子代理；review/QA 子代理使用同一映射规则，执行阶段可补充单元测试覆盖。

### 需求：JSON 兼容旧 runner 字段

系统必须保留旧 runner JSON 顶层字段，新增 observability 不得删除或改名 `run_id`、`change_name`、`status`、`stage`、`stages`、`paths`、`sessions` 和 `error`。

#### 场景：新增 observability 时保留既有 runner 顶层字段

- **对应测试**：`docs/changes/archive/2026-06-09-7-统一-wo-status-watch-极简状态输出/tests/test_status_json_observability_artifacts_contract.sh` 和 `go test ./internal/app`
- **真实数据来源**：同一真实 run state 通过 `wo status --run-id --json` 输出，现有 `internal/app` 回归测试覆盖历史 runner DTO 行为。
- **入口路径**：`internal/app.Run([]string{"status", "--run-id", runID, "--json"}, ...)` 和 `go test ./internal/app`。
- **关键断言**：旧顶层字段仍存在且值不变；`observability` 是新增字段，不替代 `paths` 或 `sessions`；既有 internal/app 回归测试通过。
- **剩余风险**：历史测试中明确禁止新增 parallel/members 字段的用例需要在执行阶段按本提案新意图更新，不能简单删除。
