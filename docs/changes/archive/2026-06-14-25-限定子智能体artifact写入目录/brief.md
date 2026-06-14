# 限定子智能体 artifact 写入目录

## 问题

`wo` 的并行 subagent 目前主要依赖最终回复中捕获裸 JSON object 来生成 member artifact。实际运行中，agent 很容易在最终回复里混入解释文字、thinking/text 分段或 markdown，导致 artifact 格式校验失败，后续 fan-in 无法消费。

用户同意不追求单文件级写限制，只要求 subagent 不破坏仓库其它文件。更合适的边界是给每个 subagent 单独划定一个 artifact 目录，让它只在该目录内写结果文件，并由 `wo` 提供确定性校验命令。

## 交付目标

- 每个 subagent 获得独立 artifact 目录，目标文件固定为 `member.json`。
- subagent prompt 明确要求写入 `ARTIFACT_PATH`，并自行运行 CLI 校验命令。
- 新增 member artifact 校验 CLI，错误信息能指导 agent 快速修正字段。
- `nodeRunSubagent` 优先读取文件 artifact，保留最终回复捕获作为兼容 fallback。
- subagent 写入边界只允许当前 member 的 artifact 目录变化，其它源码、提案和 sibling artifact 变化仍阻断。
- QA/review/implementation helper 是主阶段的证据输入；artifact 缺失或格式错误不得直接阻断主流程，主 QA/Review 决策和确定性校验才是 hard gate。

## 非目标

- 不在本提案中接入 Codex/Pi 的 OS 级文件系统 sandbox。
- 不要求限制到单个文件；允许当前 member 的专属 artifact 目录被写入。
- 不改变 fan-in 后的 parallel artifact 业务语义。

## 验收入口

```bash
bash docs/changes/25-限定子智能体artifact写入目录/tests/test_subagent_artifact_directory_contract.sh
```

## 执行上下文

先读 `internal/app/subagent.go`、`internal/app/pi.go`、`internal/app/codex.go`、`internal/app/app.go` 和 `internal/app/review_validate_command.go`。实现应优先复用现有 `readNormalizeValidateMemberArtifact` 与 member artifact schema，不重新设计 parallel artifact 格式。
