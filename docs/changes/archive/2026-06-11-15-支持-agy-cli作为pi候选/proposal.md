# 提案：支持 agy CLI 作为 Pi 候选

## 为什么

`pi` 已经承担 subagent 和部分阶段替换能力，但当前 allowlist 只有 `codex`、`pi`。`agy` CLI 的非交互参数较少，适合以低成本接入为第三个候选 agent backend，让用户后续可以用配置选择更合适的 subagent 执行器。

## 做什么

- 新增 `AgyTool`，注册名为 `agy`。
- 配置校验接受 `tool: agy` 和 `cli: agy`。
- `requiredAgentTools()` 返回 `codex`、`pi`、`agy`，sealed run 启动前统一预检。
- agy sealed runner 使用真实 CLI：`agy --print [--model M] [--conversation ID] [权限/sandbox 参数] <prompt>`。
- agy planning command 使用：`agy --prompt-interactive [--model M] <prompt>`。
- 状态和 status 展示使用现有 `sessionStateKey("agy", role)`，保证 agy 会话与 codex/pi 隔离。
- 主规格、长期回归测试和发布门禁同步声明 agy 是 Pi 的候选后端。

## 不做什么

- 不修改默认 workflow：主阶段默认仍是 `codex`，parallel member 默认仍是 `pi`。
- 不要求 agy 输出 JSONL session started 事件。
- 不让 `agy` 成为 `pi` 的别名；配置、状态 key 和错误信息必须显示真实 tool 名。
- 不绕过现有权限模型；危险权限只能在 stage 配置明确允许时映射到 `--dangerously-skip-permissions`。

## 可验证结果

- 用户配置 `tool: agy` 后，配置读取不再报未知 agent tool。
- 缺少 `agy` 时，sealed run 在创建运行态前失败，并提示安装 agy CLI。
- fake agy 记录的 argv 证明实现调用了 `--print`、`--model`、`--conversation` 和权限/sandbox 参数。
- status/state 中 agy 会话 key 使用 `agy:<role>`，不会复用 `pi:<role>`。
