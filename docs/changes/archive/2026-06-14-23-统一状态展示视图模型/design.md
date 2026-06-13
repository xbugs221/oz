# 文件目的

本文件记录状态展示视图统一的设计方案。

## 技术方案

建议分层：

- `status_view.go`：从 durable `State`/`BatchState` 构建结构化视图。
- `status_render.go`：把视图渲染为 human status、watch status、runner JSON 所需行。
- `app.go`：保留命令分发、参数解析和 stdout 写入。

对于 batch，renderer 应统一处理：

- 已有 run 的 compact stage rows。
- 未开始 change 的占位行。
- failed/blocked/aborted 提示。
- watch spinner 替换。

## 取舍

不追求一次性重写所有 status helper。先迁出 watch/status 文本拼接和 batch/run 组合逻辑，再逐步压缩历史 helper。

## 风险

- 输出文案细节容易变化。缓解方式是保留或迁移现有 status 测试，新增合同测试关注关键用户可见行和 JSON 稳定性。
- renderer 过度抽象可能难读。缓解方式是保持小函数和显式 batch/run 分支。

## 执行记录

- 执行阶段发现合同脚本的 `fd` 正则把 `.go` 过度转义为字面反斜杠匹配，导致 `status_render.go` 已存在时仍判定缺失；已修正为匹配真实 Go 源文件扩展名，断言语义不变。
