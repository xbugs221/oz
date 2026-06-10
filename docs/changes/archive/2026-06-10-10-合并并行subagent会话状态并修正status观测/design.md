# 设计

## 当前结构

并行 subagent 的产物设计本身是合理的：

```text
<runDir>/
├── parallel-members/<group>/.../*.json   # 每个 member 一个文件
├── parallel-<group>.json                 # fan-in 汇总文件
└── state.json                            # session、DAG node、阶段状态
```

问题集中在 `state.json`：

```text
runGoDAGNode
├── loadState(runID)              # 每个并发节点各自读取
├── recordGoDAGNode(running)      # 记录节点运行中
├── nodeRunSubagent(state)        # 使用旧 state 值运行 helper
│   └── saveState(state)          # 完成后整份旧 state 写回
└── recordGoDAGNode(success)      # 再记录节点完成
```

`saveState` 虽然用进程内 mutex 串行写文件，但它不会把调用方的旧快照与磁盘上的最新快照合并。并发节点保存整份旧 state 时，就会覆盖其他节点刚写入的 `sessions` 或 `dag_nodes`。

## 目标状态

新增一个小型、内部使用的状态合并边界，语义类似：

```text
mergeState(repo, runID, mutate)
├── 加锁
├── 读取最新 state.json
├── 在最新 state 上应用本次增量
├── normalize
└── 写回 state.json
```

关键点：

- `mutate` 只写本次节点拥有的字段，例如单个 session key 或单个 DAG node key。
- 合并 helper 不应在持有 `stateFileMu` 时再调用现有 `loadState/saveState`，避免重入死锁。
- `recordGoDAGNode` 应使用该 helper，只更新 `DAGNodes[nodeID]`。
- `nodeRunSubagent` 完成后应使用该 helper，只更新对应 `sessions[key]`，必要时同步当前 member 的状态增量。
- subagent runner 如果支持 progress writer，应在 backend 输出 `agent session started` 时立即合并该 session key。

## 状态流

```text
并发 member A                  并发 member B
      │                              │
      ├─ session started             ├─ session started
      │       │                      │       │
      │       └─ merge sessions[A]   │       └─ merge sessions[B]
      │                              │
      ├─ write member A artifact     ├─ write member B artifact
      │                              │
      └─ merge node/session A        └─ merge node/session B
                     │
                     ▼
          state.json 同时保留 A/B
```

## status 行为

`status/watch` 不需要从 member artifact 反推出 session。session 的权威来源仍是 `state.json.sessions`：

```text
key = "<tool>:subagent:<group>:<member-name>:<iteration>"
```

当状态合并修复后，已完成 member artifact 对应的 subagent 行自然能显示 session：

```text
执行阶段 executor-session ✓ 3.20
  代码侦察 session-code ✓ 1.10
  外部资料 session-docs ✓ 0.80
```

## 风险和取舍

- 只修复新 run。旧 run 已经丢失的 session 没有可靠来源，不能凭 artifact 重建。
- 状态合并 helper 会集中 `state.json` 写入路径，执行阶段应保持实现小而直接，避免引入复杂事务框架。
- 如果同一个 key 被同一节点重试更新，后写入的 session 应覆盖旧值；不同 key 不得互相覆盖。
