# 限定子智能体 artifact 写入目录

## 背景

并行 subagent 的 member artifact 是 `wo` 后续 fan-in、review/QA gate 和状态展示的输入。当前实现允许 read-only backend 通过最终回复返回 JSON，再由 `wo` 捕获并写入 artifact 文件。这对提示词格式过于敏感：只要 agent 在最终回复中多写分析文字、markdown 代码块，或者 backend 把回复拆成非单一 text 结构，artifact 捕获就可能失败。

用户确认可以给 subagent 一个专门目录写 artifact，只要它不能破坏其它文件即可。因此本提案把 artifact 交付方式从“乐观解析最终回复”收敛为“固定路径写文件 + CLI 自校验 + 边界检查”。

## 做什么

- 将单个 member artifact 的目标路径调整为独立目录下的 `member.json`，例如：
  `runs/<run_id>/parallel-members/<group>/<iteration>/<member-slug>.artifact/member.json`
- 在 subagent prompt 中提供 `ARTIFACT_DIR`、`ARTIFACT_PATH` 和可复制的校验命令。
- 新增 `wo validate-member-artifact`，复用现有 member artifact 规范，并输出字段级错误和修复建议。
- `nodeRunSubagent` 优先读取 subagent 写好的 `member.json`；如果文件不存在，再使用当前最终回复捕获逻辑兼容旧 backend。
- 调整 subagent 写保护：允许当前 member artifact 目录变化，但其它仓库内容或其它 member artifact 变化仍视为破坏边界。

## 为什么

固定文件交付比最终回复捕获更稳定。agent 可以在本地校验失败后自行修正 JSON，`wo` 也能得到可复核的文件路径和结构化错误。专属目录边界比单文件限制更符合现有工具能力，也能容纳未来的校验日志或辅助 evidence，而不会放开整个仓库写权限。

## 不做什么

- 不把 Codex `--sandbox` 或 Pi 文件系统 sandbox 作为硬依赖。本机实测 Codex 受限 sandbox 后端不可用，Pi 也没有等价参数。
- 不允许 subagent 修改实现源码、当前提案文件或 sibling member artifact。
- 不更改 review/QA fan-in artifact 的对外格式。
- 不让单个 helper 的 artifact 交付失败直接阻断主流程；helper 只提供证据输入，主 QA/Review agent 和确定性 validation 才承担 hard gate。
