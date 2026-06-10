# 规格

## 验收矩阵

| 需求 | 场景 | 测试 | 证据 | 风险 |
| --- | --- | --- | --- | --- |
| status/watch 直接展示提案列表 | 默认 status 不再显示 batch 或 workflow 顶部行 | `status-watch-proposal-list` | `status-watch-output-log` | 不覆盖 JSON，JSON 在非目标中保持不变 |
| status/watch 直接展示提案列表 | watch 动画只标记 running 阶段 | `status-watch-proposal-list` | `status-watch-output-log` | 只验证 running 主阶段，执行阶段可补充子代理 running 回归 |
| watch 在窄终端刷新不残留旧首行 | 长中文提案名换行后连续刷新仍只显示当前帧 | `watch-tty-clear-long-change` | `watch-tty-capture-log` | 依赖 `script` 提供伪 TTY |

### 需求：status/watch 直接展示提案列表

系统必须让 human status/watch 输出以用户关心的变更提案为第一层级，而不是以 batch/workflow 运行编号为第一层级。

#### 场景：默认 status 不再显示 batch 或 workflow 顶部行

- **给定** 当前仓库存在一个 running batch，batch 中当前提案已有 running workflow state
- **当** 用户运行默认 `wo status`
- **则** 输出第一行必须是 `- <change-name>`
- **并且** 输出中不得出现独立的 `b1 1/1` 或 `w1` 顶部 header
- **并且** 默认“正在查看最近一次批量工作流”提示不得出现在列表前
- **测试**：`docs/changes/9-精简-wo-status-watch-提案列表并修正watch清屏/tests/test_status_watch_proposal_list_contract.sh`
- **真实数据来源**：测试在临时 git 仓库和真实 `XDG_STATE_HOME` runtime 目录中写入 batch state 与 run state
- **入口路径**：`printHumanStatus` 和 `watchStatusLines`，覆盖 `wo status/watch` human 渲染核心路径
- **关键断言**：第一行是提案名；禁止 batch/workflow header；status running 阶段保留 `→`
- **剩余风险**：该场景不验证 JSON，因为 JSON 输出明确不在本次范围内

#### 场景：watch 动画只标记 running 阶段

- **给定** 当前 workflow 位于 `execution` running 状态
- **当** `wo watch` 使用某一帧 spinner 渲染状态
- **则** spinner 必须出现在 `执行阶段` 行的 marker 列
- **并且** 顶部不得显示 spinner header
- **并且** `wo status` 同一状态仍显示静态 `→`
- **测试**：`docs/changes/9-精简-wo-status-watch-提案列表并修正watch清屏/tests/test_status_watch_proposal_list_contract.sh`
- **真实数据来源**：同一份 run state 中的 `status=running`、`stage=execution`、executor session
- **入口路径**：`watchStatusLines(repo, "batch", ...)` 和 `watchStatusLines(repo, "run", ...)`
- **关键断言**：watch 中 `执行阶段 writer-session | -` 和 `执行阶段 writer-session / -` 存在；`| b1`、`/ w1` 不存在
- **剩余风险**：执行阶段可继续补充 running 子代理 marker 的细粒度单元测试

### 需求：watch 在窄终端刷新不残留旧首行

系统必须确保 `wo watch` 在真实 TTY 中连续刷新时清理完整旧帧，不能因为长提案名换行而留下上一帧首行。

#### 场景：长中文提案名换行后连续刷新仍只显示当前帧

- **给定** 一个很长的中文变更提案名，在窄终端中必然换行
- **当** 用户在伪 TTY 中运行 `wo watch` 并捕获多个刷新帧
- **则** 根据终端控制序列还原出的最终屏幕第一条业务行必须是 `- <change-name>`
- **并且** 最终屏幕不得残留 `b1`、`w1` 或旧 spinner header
- **测试**：`docs/changes/9-精简-wo-status-watch-提案列表并修正watch清屏/tests/test_watch_tty_clear_long_change_contract.sh`
- **真实数据来源**：测试构建真实 `wo` 二进制，创建临时 git 仓库、真实 runtime state 和 batch state
- **入口路径**：`script -q -c "wo watch"` 提供伪 TTY，`timeout -s INT` 捕获多个刷新帧
- **关键断言**：最终屏幕无 batch/workflow header；第一条业务行是长提案名；running 阶段仍存在 spinner marker
- **剩余风险**：不同系统的 `script` 输出前后缀可能略有差异，测试解析器会忽略非业务行
