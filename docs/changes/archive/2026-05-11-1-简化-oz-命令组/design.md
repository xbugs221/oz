## 背景

`oz` 的核心价值是把需求、设计、规格、任务和测试意图沉淀到提案目录中。命令行本身不需要暴露每个协作阶段入口；阶段动作更适合由智能体根据 skill 执行。

但下游 `wo` 工具和内置 skill 仍需要稳定的机器接口，例如 `oz validate <change> --json` 用于校验提案，`oz archive <change> --yes` 用于完成归档动作。命令组精简不能破坏这些自动化依赖。

## 目标 / 非目标

目标：

- 让 `oz --help` 呈现最小命令面。
- 让 `list` 和 `install` 支持首字母缩写。
- 让 `install` 复用现有安装内置 skill 的业务逻辑。
- 保留 `status`、`validate`、`archive` 作为下游工具和 skill 使用的自动化接口。
- 让 `plan`、`create`、`exec`、`init` 等阶段入口从用户可见接口中消失。

非目标：

- 不删除 `skills/oz-plan`、`skills/oz-create`、`skills/oz-exec`、`skills/oz-archive`。
- 不改变 `docs/changes/` 和 `docs/specs/` 的文档结构。
- 不实现交互式提案创建流程。
- 不删除或重写现有校验、状态和归档业务逻辑。
- 不创建 `.wo/runs/`，不启动 sealed run。

## 决策

命令分发保持简单的 `switch` 结构：

```text
args[0]
├── list, l          -> listCmd
├── install, i       -> installCmd
├── status           -> statusCmd
├── validate         -> validateCmd
├── archive          -> archiveCmd
├── --help, -h       -> printHelp
├── --version, -v    -> printVersion
└── other            -> unknown command
```

安装参数仅接受 `--global` 和 `-g`。`-g` 只作用于 `install` / `i`，不新增全局短参数框架。

帮助分为三类：

- 顶层帮助优先展示日常命令 `list | l` 和 `install | i`，并用独立小节标明自动化接口 `status`、`validate`、`archive`。
- `oz list --help`、`oz l -h` 展示列表用法。
- `oz install --help`、`oz i -h` 展示安装用法。
- `oz validate --help`、`oz status --help`、`oz archive --help` 保留现有用法提示，供下游工具诊断。

`init` 不保留兼容别名，安装入口统一迁移到 `install`。`plan`、`create`、`exec` 不再作为 CLI 阶段入口保留；如果后续需要恢复，应另开提案说明自动化依赖。

## 测试策略

执行阶段应在提案 `tests/` 目录中先写真实 Go 测试，再修改实现。测试重点覆盖用户可感知行为：

- 帮助文本清晰区分日常命令与自动化接口。
- `list` 与 `l` 对同一项目输出一致。
- `install` 与 `i` 复用现有 skill 安装行为。
- `--global` 与 `-g` 都安装到 `$HOME/.agents/skills/`。
- `status`、`validate`、`archive` 仍保持原有机器接口和 JSON 行为。
- `init`、`plan`、`create`、`exec` 返回失败，不再被帮助或 README 引导为 CLI 命令。

## 风险 / 取舍

- 保留 `validate`、`archive` 会让命令面不再是字面意义的“两条命令”，但这是维护下游 `wo` 自动化可靠性的必要取舍。
- 不保留 `init` 隐藏别名会导致旧脚本失败；本次选择显式失败，让命令边界清晰。
- 保留 `--version` 和 `-v` 可以继续支持发布诊断，不扩大业务命令面。
