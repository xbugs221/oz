# 设计：路径感知的运行期 git 保护

## 分类规则

把工作区变化按路径分成两类：

```text
允许的外部需求变化：
  docs/changes/<非当前 change>/**

当前 run 相关或危险变化：
  docs/changes/<当前 change>/**
  internal/**
  cmd/**
  docs/changes/archive/2026-06-11-16-允许运行中追加新需求但保留subagent写保护/tests/**
  docs/specs/**
  profiles-template/**
  prompts-template/**
  go.mod / go.sum / wo.yaml / wo.yml
  其他未明确允许的仓库路径
```

只要 diff 中同时包含允许路径和危险路径，整体按危险处理。

## 状态更新

`detectManualIntervention` 不再只看 diff 字符串是否变化。发现变化后：

- 如果全是非当前 change 目录，更新 `BaselineHead` / `BaselineDiff`，不中止。
- 如果包含当前 run 相关或危险路径，保持现有中止语义，并在错误中列出示例路径。
- `BaselineHead` 变化但 diff 只来自非当前 change 的提交，也允许继续；否则阻断。

## Subagent 写保护

parallel subagent 仍然是只读角色。执行器应在 subagent 节点前后做快照：

- subagent 只允许写自身 run artifact。
- 如果仓库业务文件变化，subagent 节点失败，fan-in 不应把它当作成功建议。
- 如果变化仅是用户新增其他 change，主工作流可以继续，但 subagent 结果不能声称自己修改了仓库。

实现可以先复用阶段边界分类，后续再把 subagent 前后快照拆成更精确的节点级保护。

## 风险

- 纯路径分类不能证明修改者身份；本提案接受这个取舍，因为目标是允许用户记录新需求，同时保留当前 run 相关路径保护。
- 如果用户在运行中修改共享配置或源码，会继续被阻断；这是防止当前 run 语义漂移所需。
- 新增需求不会自动进入已启动 batch，用户仍需显式追加。
