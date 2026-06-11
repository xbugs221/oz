# 任务

## 1. 先跑创建阶段契约测试

实现阶段观察：四个创建阶段契约测试在本轮执行时均已通过，说明目标行为已有前置实现；本轮继续整理源码边界、长期规格测试和任务状态。

- [x] 1.1 运行 `bash docs/changes/4-清理-Dagu-残留并统一-pi-subagents/tests/test_no_dagu_graph_engine_contract.sh`，确认当前实现因 Dagu/legacy 公开合同残留而失败
- [x] 1.2 运行 `bash docs/changes/4-清理-Dagu-残留并统一-pi-subagents/tests/test_default_subagent_pi_contract.sh`，确认当前实现因默认 subagent tool 仍为 `legacy-agent` 而失败
- [x] 1.3 运行 `bash docs/changes/4-清理-Dagu-残留并统一-pi-subagents/tests/test_compact_chinese_graph_and_iteration_limit.sh`，确认当前实现因默认迭代数为 30、Mermaid graph 逐轮展开且 subagent 标签中英混杂而失败
- [x] 1.4 运行 `bash docs/changes/4-清理-Dagu-残留并统一-pi-subagents/tests/test_subagent_artifact_retry_contract.sh`，确认当前实现会在 subagent 写出对象型 `evidence` 后直接失败，而不是 resume 原会话修正 artifact

## 2. 清理公开 Dagu/legacy engine 合同

- [x] 2.1 将 `workflow.engine` 配置校验收窄为空值或 `go-dag`
- [x] 2.2 更新 `wo run` 参数解析，移除或拒绝 `--engine dagu`
- [x] 2.3 移除 `wo graph --format dagu` 公开格式和错误提示中的 dagu 可选项
- [x] 2.4 保留 `json`、`mermaid` 和默认 `go-dag` 调度行为不变
- [x] 2.5 将 go-dag 仍需复用的 subagent/node helper 从 Dagu executor 语义中抽离，新增 go-dag/subagent 专属执行边界；不要把新业务规则继续加到 Dagu 残留路径上

## 3. 统一默认 subagent tool

- [x] 3.1 将 `defaultParallelConfig()` 中 planning/implementation 成员的 `Tool` 从 `legacy-agent` 改为 `pi`
- [x] 3.2 将默认 `wo.yaml` 模板中的 planning/implementation 成员 `tool` 从 `legacy-agent` 改为 `pi`
- [x] 3.3 更新现有默认配置、graph/status 相关规格测试中的期望

## 4. 收紧迭代预算和 graph 展示

- [x] 4.1 将默认 `max_review_iterations` 从 `30` 改为 `5`
- [x] 4.2 更新默认 `wo.yaml` 模板和配置测试中的迭代数期望
- [x] 4.3 将 Mermaid graph 改为紧凑循环图，不再按每轮重复生成 review/QA/fix 节点
- [x] 4.4 将 Mermaid graph 中 subagent/fan-in 可见标签改为中文，避免混入内部英文 group/type 名

## 5. 增加 go-dag subagent artifact hook

- [x] 5.1 在 subagent 正常退出并写出 `SUBAGENT_OUTPUT` 后立即执行 member artifact schema gate
- [x] 5.2 schema gate 明确拒绝对象型或嵌套型 `evidence`，并报告成员名、字段名、期望类型和 artifact 路径
- [x] 5.3 artifact schema 失败时 resume 同一 subagent session，提示词包含失败原因、期望 JSON 字段类型和“只重写 `SUBAGENT_OUTPUT`”约束
- [x] 5.4 单个 subagent artifact 修正最多重试 3 次，超过后 run 才进入 failed
- [x] 5.5 每次 subagent 尝试后仍执行只读边界检查，防止修格式时修改源码、测试、提案文档或非目标运行态
- [x] 5.6 fan-in 只汇总已经通过 member schema gate 的产物，不再承担单成员 artifact 修格式职责

## 6. 验证与归档准备

- [x] 6.1 运行本提案 `tests/` 下的四个合同测试并确认通过
- [x] 6.2 运行 `go test ./...`
- [x] 6.3 按业务逻辑把本提案测试合并进 `tests/specs/codex-workflow-cli/`
- [x] 6.4 更新 `docs/specs/codex-workflow-cli/spec.md`，删除 Dagu 公开 engine/exporter 承诺并记录唯一 `go-dag` engine、默认 5 轮、紧凑中文 graph 合同，以及 go-dag subagent artifact schema retry 合同
