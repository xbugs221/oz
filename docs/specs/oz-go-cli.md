# oz Go CLI 主规格

本文档记录 `oz` Go CLI 已归档的长期行为规格。

## 需求：命令组最小化且保留自动化接口

系统必须把 `list` 和 `install` 作为用户日常命令，同时保留 `create`、`status`、`validate`、`archive` 作为下游 `wo` 工具和内置 skill 使用的自动化接口。

### 场景：顶层帮助区分日常命令和自动化接口

- **当** 用户运行 `oz --help`
- **则** 帮助内容包含 `list | l [--json]` 和 `install | i [--global | -g]`
- **并且** 帮助内容以独立小节展示 `create`、`status <change> [--json]`、`validate <change> [--json]`、`archive <change> --yes`
- **并且** 帮助内容不包含 `init`、`plan`、`exec`

### 场景：旧阶段命令不再可执行

- **当** 用户运行 `oz init`、`oz create 需求`、`oz plan` 或 `oz exec`
- **则** 命令失败并返回非零退出码

### 场景：创建接口返回下一个提案编号

- **当** 项目中已有活动提案和归档提案
- **并且** 最大提案编号是 `53`
- **当** 下游工具运行 `oz create`
- **则** 命令输出 `54`
- **并且** 命令不得创建提案目录或提案文件

### 场景：下游校验接口继续可用

- **当** 下游工具运行 `oz validate 1-需求 --json`
- **则** 命令保持稳定 JSON 输出
- **并且** JSON 内容包含 `valid`、`change`、`errors`、`warnings` 和 `artifacts`

### 场景：下游校验接口要求验收合同

- **给定** 一个活动提案包含 `proposal.md`、`design.md`、`spec.md`、`task.md`、`tests/` 和当前 `wo` 允许的 `acceptance.json`
- **当** 下游工具运行 `oz validate <change> --json`
- **则** 命令成功并在 artifacts 中返回 `acceptance.json`
- **并且** `acceptance.json` 只需要包含 `summary`、`required_tests`、`required_evidence` 及其当前 `wo` 已支持的子字段

### 场景：下游校验接口拒绝无效验收合同

- **给定** 一个活动提案缺少 `acceptance.json`
- **当** 下游工具运行 `oz validate <change> --json`
- **则** 命令失败并指出 acceptance 合同问题
- **给定** 一个活动提案的 `acceptance.json` 包含当前 `wo` schema 不允许的字段
- **当** 下游工具运行 `oz validate <change> --json`
- **则** 命令失败并指出 schema 或 acceptance 合同问题

### 场景：下游状态和归档接口继续可用

- **当** 下游工具运行 `oz status 1-需求 --json`
- **则** 命令保持稳定 JSON 输出并包含提案产物状态
- **当** 下游工具运行 `oz archive 1-需求 --yes`
- **则** 命令继续按既有规则校验任务完成状态并归档提案
- **并且** 命令不得机械移动、改写或合并提案测试文件
- **并且** 归档后的提案测试留在 `docs/changes/archive/<date>-1-需求/tests/` 作为后续规格测试合并来源

### 场景：wo 继续通过 oz JSON 协议选择和校验 change

- **给定** `wo` 代码已经合入当前仓库
- **当** 执行器需要发现、校验或归档 change
- **则** `wo` 必须仍保留对 `oz list/status/validate/archive` 命令协议的调用能力
- **并且** 合并不得要求执行器测试改成直接 import `cmd/oz`

### 场景：归档阶段按逻辑维护长期规格测试

- **当** 智能体执行归档阶段
- **则** 智能体必须阅读归档提案中的 `tests/` 测试意图
- **并且** 按业务能力把测试用例合并到 `tests/specs/` 中稳定的规格测试文件
- **并且** 不得按提案编号或提案目录机械创建长期测试分组
- **并且** 长期规格测试可以在文件开头批注相关来源提案，便于回溯意图

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
