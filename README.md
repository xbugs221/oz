---
url: https://github.com/xbugs221/oz
---

**用 Go 语言重写的中文精简版 Openspec**

> Openspec 是一个面向 AI 协作开发的 SDD（Spec-Driven Development，规格驱动开发）工具。它的核心思路是：先把“要改什么、为什么改、验收标准是什么”写成结构化规格，再让实现围绕这些规格推进，而不是直接从一句模糊需求跳到代码。

## 适用场景

智能体编程一不小心就会变成天马行空的 vibe coding。AI 时代的编程瓶颈早已不是代码的读写速度了，而是需求是否对齐，以及变更历史是否详细可复查。`oz` 借鉴 [Openspec](https://github.com/Fission-AI/OpenSpec) 的思路，将每个需求的编码实现过程的关键信息记录下来：

- `proposal.md` 描述需求：用户痛点是什么，为什么想发起这个变更
- `design.md`：阐述思路：为实现用户需求，计划采取的方案、取舍和风险
- `spec.md` 分场景声明系统具备的行为：在特定情况下，系统应表现出什么特征
- `task.md` 任务拆解：根据计划方案和场景描述，拆解具体任务
- `tests` 测试集：符合场景声明的测试案例，编码后必须通过测试才能验收

文件树示意：

```text
docs/changes
├── archive
│   ├── 2026-04-28-1-需求描述
│   ├── ...
│   └── 2026-05-08-N-需求简述
├── N+1-需求描述
│   ├── proposal.md
│   ├── design.md
│   ├── spec.md
│   ├── task.md
│   └── tests/
└── N+2-需求描述
```

提案目录名由数字编号和中文需求描述组成，例如 `2-重写-oz-cli`。描述部分可以混用英文单词、数字和连字符，但必须至少包含一个中文汉字，不能是全英文名称。

长期主规格放在 `docs/specs/*.md`，不再为每个规格额外嵌套一层目录；文件名应直接表达能力内容，例如 `docs/specs/go-spec-driven-cli.md`。

落盘的文档和测试案例都将成为后续工作的可靠依据，也能方便协作。

## 命令

`oz` 是纯 Go 单二进制 CLI，不需要 Node.js、npm、pnpm 或 TypeScript 构建产物。当前命令边界固定为：

```text
oz plan
oz init [--global]
oz create <中文需求描述>
oz exec
oz validate <change> [--json]
oz archive <change> --yes
```

`oz create 重写-oz-go-cli` 会扫描 `docs/changes/` 和 `docs/changes/archive/`，取已有数字编号最大值加一，创建类似 `docs/changes/2-重写-oz-go-cli/` 的提案目录。提案名称可以混用英文单词、数字和连字符，但必须至少包含一个中文汉字。

`oz validate <change> --json` 输出稳定 JSON，包含 `valid`、`change`、`errors`、`warnings` 和 `artifacts` 字段。校验内容包括目录命名、四个必需文档、`spec.md` 中的需求/规范词/场景、`task.md` 任务项，以及 `tests/` 是否只包含真实测试代码。

`oz archive <change> --yes` 会先校验提案和任务完成状态，再把 `docs/changes/<change>/tests/*` 移动到项目根 `tests/`，文件名加上日期和提案来源，例如 `tests/2026-05-08-2-登录能力-archive_test.go`。如果目标测试文件已存在，归档会失败且不会覆盖旧文件；最后提案文档移动到 `docs/changes/archive/<date>-<change>/`。CLI 不自动编辑主规格；归档阶段的智能体应在 CLI 成功后读取归档目录中的 `spec.md`，合并到 `docs/specs/*.md`。

当前版本按当前工作目录解析 `docs/`，请从项目根目录运行 `oz create`、`oz validate` 和 `oz archive`。`oz archive` 要求当前提案的 `tests/` 至少包含一个真实测试文件，不支持空测试集归档。

`oz init` 会把内置的 `oz-plan`、`oz-create`、`oz-exec`、`oz-archive` 四个 skill 安装到当前项目 `.agents/skills/`。使用 `oz init --global` 时安装到 `$HOME/.agents/skills/`。支持读取 `.agents/skills/` 的智能体工具可以据此识别 `oz` 工作流。

## 一般使用流程

- `plan` 规划：讨论探索需求可行性/划定边界
- `create` 创建：根据规划阶段达成的共识，创建符合格式规定的变更提案
- `exec` 执行：引导智能体理解变更提案的格式，然后开始编程实现
- `validate` 校验：验证变更提案是否符合格式要求，支持 `--json` 结构化输出
- `archive` 归档：先用 CLI 验证并归档变更，再由智能体把归档后的 `spec.md` 合并到主规格

其中 `plan`、`create`、`exec`、`archive` 四个阶段都对应一个内置 skill 模板。在支持 skill 的系统中，先运行 `oz init` 或 `oz init --global` 安装模板；之后和智能体对话时只要提及对应关键词，智能体就能读取模板并学习如何理解 `oz` 工具、提案格式和当前阶段的职责。
