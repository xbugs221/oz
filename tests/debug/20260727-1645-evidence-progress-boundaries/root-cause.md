# Evidence 语义哈希边界过宽且无大小上限

## 问题

语义 evidence 哈希对所有 UTF-8 内容全局移除时间戳、尝试次数和
`duration_ms`，可能吞掉证书到期时间、业务事件时间或 SLA 延迟等实质变化；
同时会把大型 trace/video 整体读入内存。

## 根因

实现直接复用了 validation 日志规范化规则，未使用 acceptance 声明的 evidence
kind，也未给文本规范化设置大小边界。

## 修复

- 按 `runtime_log`、`state_snapshot` 等 kind 选择规范化策略。
- 运行日志只过滤 Go 测试耗时；状态快照只过滤明确的引擎运行字段。
- 未知 kind 和业务字段保持原文；大型 evidence 使用流式哈希。
- 大型 `runtime_log` 仍逐行过滤 Go 测试耗时，避免文件跨过 4 MiB 后把纯耗时
  变化误判为实质进展；超过扫描器单行上限的日志也以有界尾缓冲规范化。
- `state_snapshot` 只在识别出引擎记录后规范化，并要求整个文件只有一个 JSON
  文档；补齐 DAG 节点、worker、attempt 与派生路径等运行噪声，嵌套业务字段
  和尾随内容保持可见。超大快照保守视为不具备恢复资格，避免仅凭运行噪声唤醒。
- 普通 acceptance 与 stalled 恢复都从同一个已验证普通文件句柄同时计算原始
  与语义哈希，避免两次读取落在不同文件版本。
- evidence 从存在变为缺失、不可读或不再是普通文件时不计为可信进展。

## 验证

- `go test ./internal/app -run '^(TestQualityLoopEvidenceContentChangeCountsAsProgress|TestQualityEvidenceProgressHashStreamsLargeArtifacts)$' -count=1`
