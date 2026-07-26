# 设计：持续的优化会话

## 状态机

```text
execution → repair
               ↻ needs_more
               ↻ 首次 clean：强制重审
               ↓ 确认 clean
              qa
       clean ↙  ↘ needs_fix
       archive   下一轮 repair
```

`repair_N` 是可恢复的 durable stage，不把全部循环藏在一次长调用中。每轮复用 `tool:repairer` session，并输出 `repair-N.json`，至少记录本轮发现、实际修改、验证证据、剩余问题和 decision。

## 放行边界

repairer 首次决定“本轮已无已知问题”时不能直接进入 QA，系统会在同一会话强制追加一次完整重审。重审仍为 clean 才进入独立 QA；若发现问题则继续优化，下一次 clean 后再次确认。独立 QA 读取 acceptance contract、最终 diff 与最新 repair artifact；只有 QA clean 才能进入 archive。

## 配置

- 新配置使用 `stages.repair` 与 `prompts.repair`。
- `max_review_iterations` 迁移为语义更准确的 `max_repair_iterations`，默认 5，合法范围 0～10。
- `max_repair_iterations=0` 表示禁用 repair：execution 直接进入独立 QA，不生成 repair artifact；QA clean 可归档，QA needs_fix 因无修正轮次而阻塞。
- 加载旧配置时允许一次明确迁移并给出弃用提示；同时出现新旧键时拒绝歧义配置。
- sealed run 继续使用快照中的旧状态机完成，不能在 resume 时原地改写。

## 产物与兼容

新运行写 `repair-N.json`，不再生成新的 `review-N.json` 与 `fix-N-summary.md`。读取层在迁移窗口内继续识别历史产物。`oz flow status`、`graph`、`contract --json` 和 ozw 消费合同同步表达 repair 阶段。

## 风险与取舍

同一智能体存在确认偏差，因此保留独立 QA。持续会话可能积累错误假设，因此每轮 prompt 必须重新锚定 `state.json`、acceptance、完整当前 diff 和确定性验证结果；结构化检查点也不能只依赖会话记忆。
