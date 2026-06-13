# 文件目的

本文件说明统一状态展示视图模型的动机和交付内容。

## 背景

`wo status`、`wo watch` 和 `wo status --json` 都服务于同一个目标：让用户和自动化系统理解 run/batch 当前状态。当前多个入口各自拼接输出，维护成本偏高。

## 问题

输出逻辑分散会造成三类风险：

- batch 下已完成 run 与运行中 run 展示不一致。
- watch spinner 与普通 status marker 替换不一致。
- runner JSON 和 human compact rows 使用不同字段来源。

## 变更

- 新建或扩展 status renderer 层，例如 `internal/app/status_render.go`。
- renderer 从统一 `statusView` 生成 human lines、watch lines 和 JSON DTO。
- `app.go` 只负责 CLI 参数解析、目标选择和写 stdout。
- 补充状态展示回归测试。

## 稳定性原则

本提案只统一输出来源，不改变现有用户可见语义。任何文案调整必须由测试证明是既有合同要求，而不是顺手美化。
