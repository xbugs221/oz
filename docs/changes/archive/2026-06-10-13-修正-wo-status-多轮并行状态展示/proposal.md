# 提案：修正 wo status 多轮并行状态展示

## 背景

当前 `wo status` 在真实批量 run 中暴露了多处状态展示错误：

- 提案已经完成规划，run 从 execution 开始，但 `规划阶段` 仍显示 `- - -`，并展开 `需求分析/代码侦察/外部资料` 三条规划 subagent。
- execution 和 review 阶段下出现 `- 并行 implementation_context 1/2 failed -`、`- 并行 review 4/5 failed -` 这类内部 fan-in summary；其中 `failed` 只是 raw member status 不在白名单里，不代表 workflow gate 失败。
- 第三轮 review 失败后，审核阶段只显示 `x`，无法看出前两轮 review 已完成。
- review subagent 明细固定读取第一轮 session，且 subagent node lookup 可能撞上主阶段 `review_3` 节点，把真实成功的第三个 review subagent 错标为 `x`。

这些问题会让用户误判 workflow 当前状态，尤其是在 batch 队列里分不清“真实失败”“advisory 信息未标准化”和“展示聚合错误”。

## 目标

本变更收敛 human status/watch 的展示语义：

- planning_context 只是执行前上下文，不应让已规划提案看起来仍在规划。
- parallel fan-in summary 是内部 artifact 汇总，不应出现在极简 human status/watch。
- 多轮阶段的 compact marker 应能表达历史和当前轮次，例如 `✓✓x`。
- 子代理明细必须使用当前轮次，不得固定第一轮或误读主阶段 DAG node。

## 非目标

- 不改变 `wo status --json` 的既有 runner 顶层字段。
- 不改变 parallel artifact 的写入格式。
- 不改变 review gate 是否阻断、fix 是否继续、QA 是否触发的状态机规则。
- 不处理外部 agent 输出的业务质量，只处理 status 如何解释已有 durable state。

## 用户可见结果

对一个 execution 已完成、review_1/review_2 已完成、fix_1/fix_2 已完成、review_3 失败的 run，human status 应类似：

```text
- 94-收敛后端安全债务
  规划阶段 - ✓ -
  执行阶段 019... ✓ 9.11
    代码侦察 019... ✓ 2.62
    外部资料 019... ✓ 4.77
  审核阶段 019... ✓✓x 2.61
    目标核对 019... ✓ 3.06
    代码质量 019... ✓ 2.22
    测试有效 019... ✓ 3.54
    风险检查 019... ✓ 2.40
    上下文 019... ✓ 4.58
  修正阶段 019... ✓✓ 4.43
  测试阶段 - - -
  归档阶段 - - -
```

输出不得包含 `- 并行 implementation_context`、`- 并行 review`、`implementation_context`、`parallel-review`、`LGTM_WITH_MINOR_CONCERNS` 或 `completed - -` 这类 fan-in/internal/raw status 文本。

