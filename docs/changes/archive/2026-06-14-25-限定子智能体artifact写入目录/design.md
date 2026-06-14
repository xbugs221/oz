# 设计

## artifact 路径

member artifact 继续由 `memberArtifactPath` 统一生成，但路径从单个 JSON 文件迁移为专属目录下的固定文件：

```text
runs/<run_id>/parallel-members/<group>/<iteration>/<member-slug>.artifact/member.json
```

非迭代 group 可省略 `<iteration>` 层，但仍必须保留 `<member-slug>.artifact/member.json`。这样 fan-in、status 和 node artifact 仍通过同一个 helper 获取路径，不需要散落路径拼接。

## prompt 合同

subagent prompt 必须暴露：

- `ARTIFACT_DIR`
- `ARTIFACT_PATH`
- `CURRENT_CHANGE`
- `SUBAGENT_NAME`
- `SUBAGENT_PURPOSE`
- `wo validate-member-artifact --artifact "$ARTIFACT_PATH" --group <group> --member <member> --change <change-name>`

prompt 需要明确：先写文件，再运行校验命令；校验失败时按错误信息修正同一个 `member.json`。最终回复不再承担 artifact 传输职责，只需简短说明校验已通过。

## CLI 校验

新增 `wo validate-member-artifact`，参数至少包含：

- `--artifact <path>`
- `--group <group>`
- `--member <member>`
- `--change <change-name>`

实现复用 `readNormalizeValidateMemberArtifact`。错误输出需要包含字段路径、期望类型和修复提示，例如：

```text
member artifact 无效: field=evidence expected=array<string> actual=object
修复建议：请把 evidence 改成字符串数组，例如 ["已运行 pnpm test"]
```

## 写边界

执行前后仍保留 git snapshot 边界检查。允许变化范围只包括当前 subagent 的 `ARTIFACT_DIR`。如果检测到源码、`docs/changes/<current>`、其它 run artifact 或 sibling artifact 变化，节点失败并报告被破坏的路径。

## gate 语义

parallel helper 只提供证据输入，不是最终裁判。`nodeRunSubagent` 对 artifact schema 错误仍先重试修复；重试耗尽后写入一个 `status:"failed"` 的 member artifact，记录交付失败证据并让主阶段继续运行。fan-in 会把缺失 helper 记录为 `missing`，而不是让 DAG 停在单个 helper 节点。

主阶段 gate 规则：

- 写边界破坏仍是硬失败，因为它代表 helper 持久修改了非授权路径。
- Review 阶段只信任主 reviewer 的 `decision` 和 checks，不让 raw helper artifact 缺失、格式错误或 finding 直接覆盖主 reviewer。
- QA 阶段只在已有 helper artifact 中出现当前提案范围的 blocker/major finding 且主 QA 仍给出 clean 时阻断，避免主 QA 忽略明确证据。
- implementation context 只作为执行前上下文，不因 required helper 自身失败阻断 execution。

## 兼容

文件不存在时仍使用现有最终回复捕获逻辑写入 `ARTIFACT_PATH`，避免一次性切断 Pi read-only backend 或旧模型行为。后续如果外层 sandbox 可用，再把 `ARTIFACT_DIR` 接入 backend 的 writable directory。

## 风险

- 只靠 prompt 和 git snapshot 不是 OS 级强制隔离；如果 agent 写后又恢复文件，git snapshot 可能看不到瞬时破坏。本提案目标是阻止持久破坏并提升 artifact 稳定性，不声明解决恶意进程隔离。
- Pi 没有原生文件系统 sandbox。执行阶段若让 Pi 写文件，需要谨慎决定是否放开 `write` 工具；否则 Pi 可继续走最终回复捕获 fallback。
