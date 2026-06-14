# 设计

## 配置结构

旧结构：

```text
wo
└── workflow
    ├── stages
    ├── iterations
    ├── parallel
    │   └── groups
    │       ├── implementation_context
    │       ├── review
    │       └── qa
    └── validation
```

新结构：

```text
root
├── parallel
├── max_review_iterations
├── stages
│   ├── planning
│   ├── execution
│   │   └── before[]
│   ├── review
│   │   └── before[]
│   ├── qa
│   │   └── before[]
│   ├── fix
│   └── archive
├── validation
└── prompts
```

子代理不再通过 group 名表达逻辑归属。它嵌套在哪个阶段下面，就属于哪个阶段；写在 `before`，就表示在主阶段前运行。

## 字段规则

字段收敛：

```text
删除：wo
删除：workflow
删除：engine
删除：defaults
删除：iterations
删除：permissions
删除：cli
删除：tool
删除：parallel.groups
删除：mode

保留：parallel
保留：max_review_iterations
保留：stages
保留：stages.<stage>.agent
保留：stages.<stage>.model
保留：stages.<stage>.reasoning
保留：stages.<stage>.fast
保留：stages.<stage>.before
保留：stages.<stage>.before[].name
保留：stages.<stage>.before[].purpose
保留：stages.<stage>.before[].agent
保留：stages.<stage>.before[].model
保留：stages.<stage>.before[].subagent
保留：stages.<stage>.before[].required
改名：validation.max_attempts_per_stage -> validation.limit
```

`StageOptions.Tool` 可继续作为内部结构字段，避免大面积改运行时代码；YAML 输入和默认模板只暴露 `agent`。

## 子代理相关性检查

默认子代理全部启动，避免配置层做复杂条件判断。每个子代理 prompt 的开头必须先做 relevance check：

```text
先判断当前提案是否与你的职责相关。

如果无关：
- 不继续探索、搜索、运行命令或读取无关文件。
- 直接写出 JSON artifact：
  relevant=false
  irrelevant_reason=一句话说明为什么无关
  findings=[]
  evidence=只列出用于判断无关的 change 文件
- 退出。

如果相关：
- 只在你的职责范围内调查。
- 不修改源码、测试、配置或运行态。
- 输出 relevant=true 的 JSON artifact。
```

`required: true` 的成员如果返回 `relevant: false`，视为已完成必需输入；主阶段不应因为“无关”被阻断。只有 artifact 缺失、格式错误，或 `relevant: true` 且存在未处理 blocker/major，才影响主 gate。

## 默认子代理

默认保留这些子代理：

```text
execution.before
├── 代码库侦察员
└── 外部资料研究员

review.before
├── 目标核对审核员
├── 测试有效性审核员
├── 安全风险审核员
└── 上下文一致性审核员

qa.before
├── CLI/API 测试员
├── 浏览器路径测试员
└── 回归场景测试员
```

其中浏览器路径测试员在纯 CLI 提案中应快速返回 `relevant: false`，不启动浏览器、不采集截图、不跑 Playwright。

## 风险

- 这是破坏性配置变更。旧 `wo.yaml` 必须显式失败，避免用户误以为旧字段仍生效。
- prompt 必须清楚要求 relevance check，否则默认全部启动会浪费会话时间。
- `relevant: false` 必须仍是严格 JSON artifact，不能用自然语言替代，否则主阶段无法稳定读取。
