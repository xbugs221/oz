# oz Go CLI 主规格

本文档记录 `oz` Go CLI 已归档的长期行为规格。

## 需求：命令组最小化且保留自动化接口

系统必须把 `list` 和 `install` 作为用户日常命令，同时保留 `create`、`status`、`validate`、`archive` 作为下游 `oz flow` 工具和内置 skill 使用的自动化接口。

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

### 场景：下游校验接口要求验收硬合同

- **给定** 一个活动提案包含 `brief.md`、`proposal.md`、`design.md`、`spec.md`、`task.md`、`tests/` 和当前 `oz flow` 允许的 `acceptance.json`
- **当** 下游工具运行 `oz validate <change> --json`
- **则** 命令成功并在 artifacts 中返回 `brief.md` 和 `acceptance.json`
- **并且** `acceptance.json` 支持 `coverage`、`required_tests[].assertions` 和 `required_tests[].expected_initial_failure`
- **并且** `required_tests[].assertions` 必须至少包含一条业务级断言
- **并且** `coverage[].tests` 必须引用真实存在的 `required_tests[].id`
- **并且** `coverage[].evidence` 必须引用真实存在的 `required_evidence[].id`
- **并且** `required_evidence[]` 必须能追溯到 `coverage` 绑定的 `required_tests` producer
- **并且** producer 追溯规则必须由 `internal/acceptance` 统一实现，`oz validate` 和 `oz flow` 预检复用同一规则
- **并且** acceptance lifecycle 诊断边界必须由 `internal/acceptance` 统一提供，并被 `oz validate`、`oz flow` 预检、`run-acceptance` 和 QA 验收矩阵复用

### 场景：下游校验接口拒绝弱验收合同

- **给定** 一个活动提案缺少 `acceptance.json`
- **当** 下游工具运行 `oz validate <change> --json`
- **则** 命令失败并指出 acceptance 合同问题
- **给定** 一个活动提案缺少 `brief.md`
- **当** 下游工具运行 `oz validate <change> --json`
- **则** 命令失败并指出 active change artifact 问题
- **给定** 一个活动提案的 `required_tests[]` 缺少 `assertions` 或只包含 `HTTP 200` 这类弱表面断言
- **当** 下游工具运行 `oz validate <change> --json`
- **则** 命令失败并指出断言或弱验收合同问题
- **给定** 一个活动提案的 `acceptance.json` 包含真正未知的字段
- **当** 下游工具运行 `oz validate <change> --json`
- **则** 命令失败并指出 schema 或 acceptance 合同问题
- **给定** 一个活动提案的 `coverage` 引用了不存在的测试或证据 id
- **当** 下游工具运行 `oz validate <change> --json`
- **则** 命令失败并指出引用错误

### 场景：required_tests 执行证据链可追溯

- **给定** 一个活动提案的合法 `acceptance.json` 使用旧必填字段集合，并声明 `coverage`、`required_tests` 和 `required_evidence`
- **并且** required test 真实写入 runtime evidence
- **当** 下游工具运行 `oz flow run-acceptance --change <change> --json`
- **则** 命令保持既有 `summary`、`tests`、`evidence` 和 `diagnostics` 输出兼容
- **并且** JSON 结果必须包含 required evidence 到 required tests 的 coverage 绑定
- **并且** JSON 结果必须包含 producer 追溯结果，说明对应 evidence 已由 coverage 绑定的 required test 验证

### 场景：下游状态和归档接口继续可用

- **当** 下游工具运行 `oz status 1-需求 --json`
- **则** 命令保持稳定 JSON 输出并包含提案产物状态
- **并且** artifacts 必须包含 `brief.md`
- **并且** acceptance 摘要必须暴露 `required_tests`、`required_evidence` 和 `coverage` 的数量
- **当** 下游工具运行 `oz archive 1-需求 --yes`
- **则** 命令继续按既有规则校验任务完成状态并归档提案
- **并且** 命令不得机械移动、改写或合并提案测试文件
- **并且** 归档后的提案测试留在 `docs/changes/archive/<date>-1-需求/tests/` 作为后续规格测试合并来源

### 场景：oz flow 继续通过 oz JSON 协议选择和校验 change

- **给定** `oz flow` 代码已经合入当前仓库
- **当** 执行器需要发现、校验或归档 change
- **则** `oz flow` 必须仍保留对 `oz list/status/validate/archive` 命令协议的调用能力
- **并且** 合并不得要求执行器测试改成直接 import `cmd/oz`

### 场景：oz flow 命令分发边界保持清晰

- **给定** `oz flow` CLI 代码已经合入当前仓库
- **当** 维护者调整 repo 命令分发、无参数交互流程或 planning 入口
- **则** `internal/app/command_dispatch.go`、`internal/app/interactive.go` 和 `internal/app/planning.go` 必须作为独立边界文件存在
- **并且** `internal/app/app.go` 不得重新直接包含 `run`、`resume`、`batch`、`restart`、`status`、`abort`、`clean`、`watch`、`--resume`、`--run` 这些 repo 命令的大 switch case
- **并且** `internal/app` 与 `cmd/oz` 命令面回归必须通过，证明拆分不改变现有 CLI 行为

### 场景：standalone oz CLI 命令边界保持清晰

- **当** 维护者调整 standalone `oz` CLI 的入口、安装、提案查询、校验或归档命令
- **则** `internal/ozcli/cli.go`、`internal/ozcli/cmd_install.go`、`internal/ozcli/cmd_change.go`、`internal/ozcli/cmd_validate.go` 和 `internal/ozcli/cmd_archive.go` 必须作为独立边界文件存在
- **并且** `Main/run`、`installCmd/printInstallHelp`、`listCmd/createCmd/statusCmd`、`validateCmd/validateChange/validateAcceptanceFiles`、`archiveCmd/ensureTasksDone` 必须分别落在对应职责文件
- **并且** `internal/ozcli/ozcli.go` 不得重新成为 700 行以上的混合职责文件
- **并且** `internal/ozcli` Go 回归必须通过，证明拆分不改变现有 CLI 行为

### 场景：工作流配置解析边界保持清晰

- **当** 维护者调整 `oz-flow.yaml` schema、profile 模板、parallel 展开或 validation 配置解析
- **则** `internal/app/config_schema.go`、`internal/app/config_profiles.go`、`internal/app/config_parallel.go` 和 `internal/app/config_validation.go` 必须作为独立边界文件存在
- **并且** `internal/app/config.go` 不得重新直接定义 schema input、profile 渲染、parallel 展开或 validation 解析 helper
- **并且** 默认 tree config、legacy 字段拒绝、profile 发现、MADA profile 生成和 parallel 配置合同必须继续通过，证明用户可见配置行为不变

### 场景：工作流 Engine 运行边界保持清晰

- **当** 维护者调整 workflow Engine 的状态模型、运行入口、恢复逻辑、阶段执行或进度持久化
- **则** `internal/app/state_model.go`、`internal/app/engine_run.go`、`internal/app/engine_resume.go`、`internal/app/engine_stage.go` 和 `internal/app/engine_progress.go` 必须作为独立边界文件存在
- **并且** `State` 模型、`resumeRun`、`runLoop`、`runStage`、`stageProgressWriter` 必须分别落在对应职责文件
- **并且** `internal/app/state.go` 不得重新成为承载核心运行职责的 1000 行级混合文件
- **并且** workflow 相关 Go 回归必须继续通过，证明文件拆分不改变运行行为

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
