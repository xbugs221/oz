# oz Go CLI 主规格

本文档记录 `oz` Go CLI 已归档的长期行为规格。

## 需求：命令组最小化且保留自动化接口

系统必须把 `list` 和 `install` 作为用户日常命令，同时保留 `status`、`validate`、`archive` 作为下游 `wo` 工具和内置 skill 使用的自动化接口。

### 场景：顶层帮助区分日常命令和自动化接口

- **当** 用户运行 `oz --help`
- **则** 帮助内容包含 `list | l [--json]` 和 `install | i [--global | -g]`
- **并且** 帮助内容以独立小节展示 `status <change> [--json]`、`validate <change> [--json]`、`archive <change> --yes`
- **并且** 帮助内容不包含 `init`、`plan`、`create`、`exec`

### 场景：旧阶段命令不再可执行

- **当** 用户运行 `oz init`、`oz create 需求`、`oz plan` 或 `oz exec`
- **则** 命令失败并返回非零退出码

### 场景：下游校验接口继续可用

- **当** 下游工具运行 `oz validate 1-需求 --json`
- **则** 命令保持稳定 JSON 输出
- **并且** JSON 内容包含 `valid`、`change`、`errors`、`warnings` 和 `artifacts`

### 场景：下游状态和归档接口继续可用

- **当** 下游工具运行 `oz status 1-需求 --json`
- **则** 命令保持稳定 JSON 输出并包含提案产物状态
- **当** 下游工具运行 `oz archive 1-需求 --yes`
- **则** 命令继续按既有规则校验任务完成状态、移动测试文件并归档提案

## 需求：列表命令支持缩写

系统必须让 `oz l` 成为 `oz list` 的等价命令，方便用户快速查看活动提案。

### 场景：缩写列表输出与完整命令一致

- **当** 同一个项目中存在活动提案和归档提案
- **并且** 用户分别运行 `oz list --json` 和 `oz l --json`
- **则** 两个命令都只输出活动提案
- **并且** 两个命令的 JSON 内容一致

### 场景：列表命令提供对应帮助

- **当** 用户运行 `oz list --help` 或 `oz l -h`
- **则** 帮助内容展示 `oz list [--json]` 和 `oz l [--json]` 的用法

## 需求：安装命令支持缩写和全局短参数

系统必须通过 `install` 命令安装内置 skill，并允许使用 `i` 和 `-g` 缩写。

### 场景：本地安装 skill

- **当** 用户在项目根目录运行 `oz install`
- **则** 内置 skill 被安装到项目 `.agents/skills/` 目录

### 场景：全局安装 skill

- **当** 用户运行 `oz install --global`、`oz i --global` 或 `oz i -g`
- **则** 内置 skill 被安装到 `$HOME/.agents/skills/` 目录

### 场景：安装命令提供对应帮助

- **当** 用户运行 `oz install --help` 或 `oz i -h`
- **则** 帮助内容展示 `oz install [--global | -g]` 和 `oz i [--global | -g]` 的用法
