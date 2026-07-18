# 规格：支持 agy CLI 作为 Pi 候选

## 验收矩阵

| 场景 | required_tests | required_evidence |
| --- | --- | --- |
| 需求：配置支持 agy 候选后端 / 场景：stage 和 parallel member 可选择 agy | `contract-agy-config-preflight` | `agy-config-preflight-log` |
| 需求：启动前检查 agy 可用性 / 场景：缺少 agy 时不创建运行态 | `contract-agy-config-preflight` | `agy-config-preflight-log`, `agy-state-snapshot` |
| 需求：agy runner 使用真实 CLI 参数 / 场景：sealed run 和 planning 参数映射正确 | `contract-agy-cli-args` | `agy-cli-args-log` |

### 需求：配置支持 agy 候选后端

系统必须把 `agy` 作为独立 agent tool 支持。用户可以在 workflow stages 中写 `tool: agy` 或 `cli: agy`，也可以在 parallel member 中写 `tool: agy`。默认主阶段仍是 `codex`，默认 parallel member 仍是 `pi`。

#### 场景：stage 和 parallel member 可选择 agy

- **给定** `wo.yaml` 中 execution 阶段配置 `tool: agy`
- **且** parallel implementation_context 成员配置 `tool: agy`
- **当** 用户启动 sealed run
- **则** 配置读取不得报 `未知 agent tool "agy"`
- **且** 状态 key 必须按 `agy:<role>` 隔离，不得写入 `pi:<role>`
- **对应测试**：`docs/changes/archive/2026-06-11-15-支持-agy-cli作为pi候选/tests/test_agy_config_preflight_contract.sh`
- **真实数据来源**：临时仓库中的真实 `wo.yaml` 和真实 `wo run` CLI 入口
- **入口路径**：`wo run --change <change> --json`
- **关键断言**：配置接受 agy；执行失败时错误不是未知工具；state/session key 不把 agy 写成 pi
- **剩余风险**：契约测试使用 fake agy，不验证真实 agy 账号权限

### 需求：启动前检查 agy 可用性

sealed run 创建任何运行态之前，必须检查 `codex`、`pi`、`agy` 都存在。缺少 `agy` 时应直接失败，输出包含 `agy` 和安装指引。

#### 场景：缺少 agy 时不创建运行态

- **给定** 临时 PATH 中只有 fake `codex` 和 fake `pi`
- **且** 没有 `agy`
- **当** 用户启动 sealed run
- **则** 命令必须失败
- **且** 输出明确提到缺少 `agy` 和安装指引
- **且** 用户状态目录不得创建 run state
- **对应测试**：`docs/changes/archive/2026-06-11-15-支持-agy-cli作为pi候选/tests/test_agy_config_preflight_contract.sh`
- **真实数据来源**：临时 PATH、临时用户状态目录、真实 `wo run` CLI 入口
- **入口路径**：`wo run --change <change> --json`
- **关键断言**：缺少 agy 时失败；错误含 agy；没有 run state
- **剩余风险**：不验证安装 agy 的具体方式

### 需求：agy runner 使用真实 CLI 参数

系统必须通过 Go `exec.Command` 参数数组调用 agy，不能 shell 拼接 prompt。sealed run 使用 `--print`，planning 使用 `--prompt-interactive`。恢复已知会话时使用 `--conversation <sessionID>`。

#### 场景：sealed run 和 planning 参数映射正确

- **给定** fake `agy` 会把 argv 写入日志
- **当** execution 阶段配置 `tool: agy`、`model: agy-model`、`permissions: danger-full-access`
- **且** 已有 `agy:executor` session id 为 `conv-123`
- **则** sealed run 调用必须包含 `--print`、`--model agy-model`、`--conversation conv-123`、`--dangerously-skip-permissions`
- **且** prompt 作为单个参数传入，不能被 shell 拆分
- **当** planning 阶段配置 `tool: agy`
- **则** planning command 必须包含 `--prompt-interactive`
- **对应测试**：`docs/changes/archive/2026-06-11-15-支持-agy-cli作为pi候选/tests/test_agy_cli_args_contract.sh`
- **真实数据来源**：fake agy 记录的真实 argv、Go 测试调用的真实 runner/planning command 构造
- **入口路径**：`go test ./tests/app`
- **关键断言**：argv 包含 agy 参数；session 使用 conversation；prompt 未被 shell 拆分；状态 key 使用 agy
- **剩余风险**：fake agy 不验证真实 agy 服务端是否接受模型名
