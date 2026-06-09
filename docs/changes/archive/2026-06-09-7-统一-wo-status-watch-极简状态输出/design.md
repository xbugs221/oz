# 设计

## 统一状态视图

新增一层状态视图模型，作为 human 输出和 JSON observability 的共同来源：

```text
statusView
  display_id
  indicator
  rows[]
  artifacts

statusRow
  kind              stage | subagent
  name              规划阶段 / 代码侦察
  full_name         原始阶段名或配置中的完整 subagent 名称
  stage             planning / execution / review_1 / qa_1 / archive
  group             implementation_context / review / qa
  session_id
  marker            - / → / ✓ / ✓✓ / x
  duration_minutes
  indent
  artifacts
```

`wo status`、`wo watch`、batch 内 workflow 展示和 `--json` 都从这组 row 派生，避免再维护多套渲染逻辑。

## Human 输出规则

单 workflow 第一行固定为：

```text
<indicator> <display-id>
```

`display-id` 使用当前解析到的短编号，例如 `w1`。短编号比真实 timestamp run id 更适合人类持续观察；真实 run id 继续保留在 JSON。

正文每行固定四列：

```text
<名称> <会话id> <进度标记> <耗时分钟>
```

字段规则：

- 无会话或无耗时写 `-`，不得省略列。
- 耗时统一为分钟浮点数，格式固定为两位小数，不写单位。
- `status` 中 header indicator 使用 `→`；`watch` 中 header indicator 使用 spinner `| / - \`。
- 正在运行的主阶段行仍使用 `→`，不使用 spinner，避免每秒改变正文宽度。
- 完成轮次用勾数量表示，例如审核两轮完成可显示 `✓✓`。
- 失败、阻塞或关键 artifact 缺失显示 `x`。

## 名称映射

主阶段名称保持四字左右：

| 内部阶段 | 显示名称 |
| --- | --- |
| planning | 规划阶段 |
| execution | 执行阶段 |
| review_N | 审核阶段 |
| fix_N | 修正阶段 |
| qa_N | 测试阶段 |
| archive | 归档阶段 |

默认 subagent 名称使用短名：

| 完整名称 | 短名 |
| --- | --- |
| 需求分析员 | 需求分析 |
| 代码库侦察员 | 代码侦察 |
| 外部资料研究员 | 外部资料 |
| 目标核对审核员 | 目标核对 |
| 代码质量审核员 | 代码质量 |
| 测试有效性审核员 | 测试有效 |
| 安全风险审核员 | 风险检查 |
| 上下文一致性审核员 | 上下文 |
| CLI/API 测试员 | CLI/API |
| 浏览器路径测试员 | 浏览器 |
| 证据采集员 | 证据采集 |
| 回归场景测试员 | 回归场景 |

非默认名称先移除 `员`、`研究员`、`审核员`、`测试员` 等后缀；仍过长时保留前四个中文字符或完整 ASCII token。该规则只影响 human 显示，JSON 保留 `full_name`。

## 子代理行来源

子代理本身是进程，不能只从 fan-in 汇总 artifact 推断。状态视图必须综合以下来源：

- `state.Workflow.Parallel.Groups`：确定配置中应显示的成员和所属主阶段。
- `state.Sessions["<tool>:subagent:<group>:<member>:<iteration>"]`：确定子代理会话 id。
- `state.DAGNodes`：确定子代理节点状态、开始时间、结束时间和成员 artifact 路径。
- `parallel-members/<group>/<iteration>/<member>.json`：确定成员结果。
- `parallel-*.json` fan-in artifact：确定 group 汇总产物路径，但不在人类输出中显示汇总行。

如果成员已经进入 DAG node 但 artifact 缺失或非法，行显示 `x`，JSON 中仍给出预期路径。

## JSON observability

`wo status --run-id <run-id> --json` 继续输出既有字段：

```json
{
  "run_id": "...",
  "change_name": "...",
  "status": "...",
  "stage": "...",
  "stages": {},
  "paths": {},
  "sessions": {},
  "error": ""
}
```

新增 `observability` 字段：

```json
{
  "observability": {
    "display_id": "w1",
    "engine": "go-dag",
    "rows": [
      {
        "kind": "stage",
        "name": "执行阶段",
        "full_name": "execution",
        "stage": "execution",
        "session_id": "writer-session",
        "marker": "→",
        "duration_minutes": 6.5,
        "artifacts": {
          "stage_artifact": "/repo/docs/changes/7-统一输出/task.md"
        }
      },
      {
        "kind": "subagent",
        "name": "代码侦察",
        "full_name": "代码库侦察员",
        "stage": "execution",
        "group": "implementation_context",
        "session_id": "subagent-session-1",
        "marker": "✓",
        "duration_minutes": 1.1,
        "artifacts": {
          "member_artifact": "/state/runs/<run-id>/parallel-members/implementation_context/code.json",
          "group_artifact": "/state/runs/<run-id>/parallel-implementation-context.json"
        }
      }
    ],
    "artifacts": {
      "run_state": "/state/runs/<run-id>/state.json",
      "change_proposal": "/repo/docs/changes/<change>/proposal.md",
      "change_design": "/repo/docs/changes/<change>/design.md",
      "change_spec": "/repo/docs/changes/<change>/spec.md",
      "change_task": "/repo/docs/changes/<change>/task.md",
      "change_acceptance": "/repo/docs/changes/<change>/acceptance.json"
    }
  }
}
```

所有路径使用绝对路径，方便下游工具直接读取。缺失的预期产物也应给出路径，不能因为文件不存在就省略。

## 风险和取舍

- Human 输出会比当前版本少显示 change name 和 engine；这是为了满足极简观察目标。机器需要的信息进入 JSON。
- `--json` 新增字段会改变此前“不新增 parallel 结构”的历史约束；这是用户明确提出的新需求。旧字段不得删除，执行阶段需要同步更新旧测试的期望。
- 当前仓库缺少 `cmd/wo` 入口，因此创建阶段的契约测试覆盖 `internal/app.Run` 和渲染函数。等入口恢复后，执行阶段应补一条 CLI 二进制级回归测试。
