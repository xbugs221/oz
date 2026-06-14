# 树状简化 wo 配置

## 问题

当前 `wo.yaml` 暴露了太多实现形状：

- 顶层 `wo.workflow` 多一层包装。
- `cli` 和 `tool` 表达同一件事。
- `permissions` 暴露给用户，但权限边界应该由 workflow 按主阶段和只读子代理场景内部决定。
- `parallel.groups.<group>` 要求用户记住 group 名和主阶段的挂载关系。
- `iterations` 让用户提前配置具体轮次，和“失败后自动升级思考深度”的运行时策略重复。
- 默认配置每个阶段重复写默认值，增加维护成本。

子代理本质上也是阶段内部发起的会话。配置应长得像 `wo status` 的树状层级，而不是把主阶段和并行 helper 分散到两套结构里。

## 目标

采用新的 KISS 配置格式：

```yaml
parallel: true
max_review_iterations: 5

stages:
  planning:
    agent: codex
    reasoning: xhigh

  execution:
    agent: codex
    reasoning: low
    model: gpt-5.1-codex
    before:
      - name: 代码库侦察员
        purpose: 搜索相关源码、测试、配置和既有实现约定
        agent: pi
        subagent: explore
        required: false

  review:
    agent: codex
    reasoning: high
    before:
      - name: 目标核对审核员
        purpose: 核对实现是否满足 proposal/spec/task/acceptance
        agent: pi
        required: true

  qa:
    agent: codex
    reasoning: high
    before:
      - name: 浏览器路径测试员
        purpose: 执行页面真实用户路径
        agent: pi
        model: pi-browser-model
        required: true

  fix:
    agent: codex
    reasoning: low

  archive:
    agent: codex
    reasoning: low

validation:
  limit: 3
  commands: []

prompts:
  execution: |
    ...
```

核心语义：

- `parallel: false` 时，任何 `stages.<stage>.before` 子代理都不启动。
- `stages.<stage>.before` 表示当前主阶段运行前启动的只读辅助会话。
- `required: true` 表示必须产出格式有效 artifact；artifact 可以是 `relevant: false`。
- `required: false` 表示尽力产出 artifact，失败只记录 warning，不阻断主阶段。
- `model` 只在非空时传给对应 agent CLI。
- 连续失败两次后的思考深度升级由运行时自动完成，不再提供 `iterations` 配置。

## 非目标

- 不兼容旧 `wo.workflow` 格式。
- 不兼容 `cli` 或 `tool` 别名。
- 不提供旧配置自动迁移。
- 不开放 `permissions` 配置。
- 不新增模型名合法性检查。
- 不让子代理直接写源码、测试或工作流状态。
