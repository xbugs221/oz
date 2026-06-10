# 合并并行 subagent 会话状态并修正 status 观测

## 问题

`go-dag` 会并发运行 planning、implementation、review、qa 的 parallel subagents。每个 subagent 都会写自己的 member artifact，fan-in 再生成一个 group 汇总文件。当前问题不在 artifact 数量，而在 durable state 的合并方式：

```text
subagent A 读取 state.json
subagent B 读取 state.json
A 完成后保存 sessions={A}
B 完成后用旧快照保存 sessions={B}
最终 state.json 只剩 B，A 的 session 被覆盖
```

结果是用户看到类似下面的状态：

```text
审核阶段 reviewer-session → 2.47
  目标核对 - ✓ -
  代码质量 - ✓ -
  测试有效 - ✓ -
  风险检查 - ✓ -
  上下文 context-session ✓ -
```

这些 `- ✓ -` 行通常不是 subagent 没有执行，而是 session 在并发保存 `state.json` 时丢失。主流程仍能推进，是因为推进依赖 member artifact 和 fan-in artifact，不依赖 `sessions` 完整。

## 目标

- 让并行 subagent 保存 session 时只合并自己的状态增量，不覆盖其他节点刚写入的状态。
- 让 subagent running 期间的 session started 事件也能进入 `state.json`。
- 让 status/watch 展示与真实执行结果一致：完成的 member artifact 对应的 session 不应随机缺失。

## 影响范围

- `internal/app/subagent.go`：subagent session 记录和 running progress 记录。
- `internal/app/go_dag.go`：DAG node 状态记录需要与最新 state 合并。
- `internal/app/state.go`：新增或调整持久化状态的原子合并边界。
- `internal/app/status_view.go`：保持以 `state.Sessions` 为权威来源，验证完成 member 行能显示 session。

## 不做什么

- 不改变 `parallel-members/**.json` 的 schema。
- 不改变 `parallel-*.json` fan-in 汇总 schema。
- 不改变 review/QA/fix 的业务判断和 gate 规则。
- 不为历史 run 追补已经被覆盖掉的 session。

## 验收

本提案包含一个合同测试脚本，使用真实 `internal/app` 包级测试入口创建临时 git 仓库、真实 run state、真实 member artifact 和真实 status view。执行阶段必须让该测试通过：

```bash
bash docs/changes/10-合并并行subagent会话状态并修正status观测/tests/test_parallel_subagent_session_state_contract.sh
```
