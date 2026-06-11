# 设计：agy 作为独立 AgentTool

## 后端模型

`agy` 使用独立 `AgyTool`，不复用 `PiTool`。这样配置校验、状态 key、错误信息、预检和后续参数差异都能保持清晰。

```text
AgentRegistry
  - CodexTool
  - PiTool
  - AgyTool

validAgentTool: codex | pi | agy
requiredAgentTools: codex, pi, agy
session key: agy:<role>
```

## 参数映射

sealed run：

```text
agy --print [--model <model>] [--conversation <sessionID>] [permission flags] <prompt>
```

planning：

```text
agy --prompt-interactive [--model <model>] <prompt>
```

`StageOptions.Reasoning` 对 agy 没有对应参数，本次忽略，不报错。`StageOptions.Fast` 也不映射到 agy。权限映射保持最小：

- `permissions: danger-full-access` 或同等已有危险权限值时追加 `--dangerously-skip-permissions`。
- `permissions: sandbox` 或已有受限权限值时追加 `--sandbox`。
- 默认权限不追加权限参数。

## 会话策略

agy help 暂未显示 JSONL session 输出。首次执行如果无法从 stdout/stderr 稳定提取 conversation id，就返回空 session id；恢复已知会话时把现有 `sessionID` 传给 `--conversation`。状态层仍然按 `agy:<role>` 隔离 key，避免误用 `pi` session。

如果后续确认 agy 有稳定会话输出格式，可在不改变配置面的前提下补充解析。

## 测试策略

创建阶段提供两条契约测试：

- 配置和预检测试：证明 `agy` 可作为 stage 和 parallel member tool，且缺失 agy 时 sealed run 不创建状态。
- 参数映射测试：用 fake agy 捕获 argv，证明 sealed runner 和 planning command 使用正确参数，不 shell 拼接 prompt，不把 agy 当成 pi。

执行阶段应把这些契约沉淀为 `tests/specs/codex-workflow-cli/` 或 `tests/app/` 下的长期回归测试。

## 风险

- agy 新版本可能调整参数名；本次以本机 `agy --help` 输出为准。
- 无稳定 session 输出会降低自动 resume 能力；本次只保证已知 sessionID 能传入 `--conversation`。
- 启动前强制检查 `agy` 会增加安装前置条件；这是当前 `codex/pi` 预检策略的自然延伸。
