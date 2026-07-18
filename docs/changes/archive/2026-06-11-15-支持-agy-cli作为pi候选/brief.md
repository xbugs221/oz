# 简报：支持 agy CLI 作为 Pi 候选

## 用户问题

当前工作流只允许 `codex` 和 `pi` 两个 agent backend。`pi` 主要承担 subagent 和可替换阶段执行，但用户已经在本机安装了参数较少的 `agy` CLI，希望把它接入为 `pi` 的候选后端，用于后续按配置替换部分阶段或 subagent。

## 交付目标

- agent tool allowlist 增加 `agy`。
- 用户可在 workflow stages 和 parallel members 中配置 `tool: agy` 或 `cli: agy`。
- sealed run 调用 `agy --print` 执行一次性提示词，按 `--model`、`--conversation`、权限和 sandbox 配置映射参数。
- planning 交互入口调用 `agy --prompt-interactive`，保持人工规划可接续。
- agy 作为候选后端，不改变默认主阶段 `codex` 和默认 subagent `pi`。
- 启动 sealed run 前把 `agy` 纳入 CLI 存在性检查，缺失时提示用户安装，不创建 run state。

## 非目标

- 不把 `agy` 设置为默认后端。
- 不修改 `pi` 的命令行参数和 JSONL session 解析逻辑。
- 不实现 agy 专属 profile 或自动根据模型选择后端。
- 不承诺 agy 输出 JSONL；本次只要求进程成功、stderr 受限上报、可用 `--conversation` 恢复已知会话。

## 验收入口

执行阶段必须先运行本提案 `docs/changes/archive/2026-06-11-15-支持-agy-cli作为pi候选/tests/` 下的契约测试，确认当前实现因不认识 `agy`、未预检 agy、没有 agy runner 和文档未同步而失败。实现后这些测试必须通过，并补充根目录长期回归测试。

## 执行阶段默认上下文

`agy --help` 在本机显示支持 `--print`、`--prompt-interactive`、`--conversation`、`--model`、`--dangerously-skip-permissions`、`--sandbox`、`--add-dir`、`--print-timeout` 等参数。执行器应优先复用现有 `AgentTool` / `AgentRunner` / `StageOptions` / `sessionStateKey` 抽象，不新增并行调度抽象。
