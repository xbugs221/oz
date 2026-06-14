---
url: https://github.com/xbugs221/oz
---

**中文精简版 Openspec 规范工具和工作流执行器**

## 适用场景

智能体编程一不小心就会变成天马行空的 vibe coding。AI 时代的编程瓶颈早已不是代码的读写速度了，而是需求是否对齐，以及变更历史是否详细可复查。

`oz` 仓库同时维护两个命令：`oz` 负责提案规范、skill 安装、校验和归档；`oz flow` 负责按这些提案运行自动化工作流。两个命令来自同一个 Go module、同一个源码 checkout 和同一个 release 批次，避免规范工具与执行器版本错位。

`oz` 借鉴 [Openspec](https://github.com/Fission-AI/OpenSpec) 的思路，将每个需求的编码实现过程的关键信息记录下来：

- `proposal.md` 描述需求：用户痛点是什么，为什么想发起这个变更
- `design.md`：阐述思路：为实现用户需求，计划采取的方案、取舍和风险
- `spec.md` 分场景声明系统具备的行为：在特定情况下，系统应表现出什么特征
- `task.md` 任务拆解：根据计划方案和场景描述，拆解具体任务
- `acceptance.json` 结构化验收合同：列出必须通过的测试和 QA 必须采集的截图、trace、network、runtime log 等证据
- `tests` 测试集：符合场景声明的契约测试，编码后必须通过

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
│   ├── acceptance.json
│   └── tests/
└── N+2-需求描述
```

提案目录名由数字编号和中文需求描述组成，例如 `2-重写-oz-cli`。描述部分可以混用英文单词、数字和连字符，但必须至少包含一个中文汉字，不能是全英文名称。

长期主规格放在 `docs/specs/*.md`；文件名应直接表达能力内容，例如 `docs/specs/go-spec-driven-cli.md`。长期规格测试放在 `tests/specs/`，按业务能力合并维护，不按提案编号或提案目录机械分组；测试文件开头可以批注来源提案，便于回溯意图。

## 一般使用流程

`oz` 和 `oz flow` 都是 Go 编译的二进制程序，windows/macos/linux 三大操作系统兼容，而且 arm/x64 兼容。

安装最新版：

```bash
go install github.com/xbugs221/oz@latest
```

提案的一般使用流程涉及 4 个阶段：

- `plan` 规划：讨论探索需求可行性/划定边界
- `create` 创建：根据规划阶段达成的共识，创建符合格式规定的变更提案
- `exec` 执行：引导智能体理解变更提案的格式，然后开始编程实现
- `archive` 归档：先用 CLI 验证并归档变更，再由智能体把归档后的 `spec.md` 合并到主规格，并把归档提案中的测试按业务逻辑合并到 `tests/specs/`

各阶段都对应一个内置 skill ，先运行 `oz install --global` 安装模板；之后和智能体对话时只要提及对应关键词，智能体就能读取模板并学习如何理解 `oz` 工具、提案格式和当前阶段的职责。

创建阶段的目录编号由 CLI 计算：运行 `oz create` 会输出下一个提案数字，例如已有最大编号 `53` 时输出 `54`。这个命令只提供编号，不创建提案文件；智能体继续按 `oz-create` skill 创建 `proposal.md`、`design.md`、`spec.md`、`task.md`、`acceptance.json` 和 `tests/`。

`oz flow config` 会在仓库根目录生成 `oz-flow.yaml`；`oz flow config --global` 会生成 `~/oz-flow.yaml`：

```yaml
parallel: true
max_review_iterations: 5
stages:
  planning:
    agent: codex
    reasoning: xhigh
  execution:
    agent: codex
    reasoning: low
  fix:
    agent: codex
    reasoning: low
  review:
    agent: codex
    reasoning: high
  qa:
    agent: codex
    reasoning: high
  archive:
    agent: codex
    reasoning: low
validation:
  limit: 3
  commands: []
prompts:
  planning: |
    调用 `oz-plan` 技能开始讨论规划阶段
```

配置按 `内置默认 -> ~/oz-flow.yaml -> 仓库 oz-flow.yaml -> run 快照` 合并；`validation.commands` 为空时不运行门禁，填入项目真实命令时使用 `executable` 和 `args` 描述 argv，`execution` 和 `fix` 阶段完成后会直接执行该程序，不经过 shell。

## GitHub Actions 门禁

仓库使用 GitHub Actions 运行 CI 和 Release 门禁。`CI` workflow 会在 push 和 pull request 时从当前 checkout 构建本地 `oz`、`oz flow`，然后运行：

```bash
go test ./...
```

`Release` workflow 在 `v*` tag 或手动触发时复用同一套 Go 测试门禁，只有 `go test ./...` 通过后，才进入跨平台构建、校验和 GitHub Release 上传。提案合同和长期 shell 规格测试按对应变更或审查任务定向运行，不由 Release workflow 盲目遍历历史脚本。

复现 GitHub CI 失败时，先在仓库根目录运行上面的两类命令；如果失败发生在 `Run Go tests`，优先用 GitHub run 页面里的失败包和测试名缩小到定向 `go test` 命令，再对照相关 prompt、长期规格和提案测试检查合同是否一致。

## 批注

> Openspec 是一个面向 AI 协作开发的 SDD（Spec-Driven Development，规格驱动开发）工具。它的核心思路是：先把“要改什么、为什么改、验收标准是什么”写成结构化文档，再让编程人员围绕这些规格推进，而不是直接根据一句临时模糊需求随意修改代码。落盘的文档和测试案例都将成为后续工作的可靠依据，也能方便协作
`plan`、`exec`不再是 CLI 阶段命令；`create` 只作为编号查询接口保留，不负责创建阶段产物。创建、规划、执行阶段由智能体读取内置 skill 后完成。`status`、`validate`、`archive` 虽可直接运行，但定位是下游自动化接口，不作为日常用户入口。

从旧 `oz flow` 仓库迁移时，请先完成或放弃旧仓库路径下未结束的运行态。`oz flow` 的用户状态按仓库绝对路径隔离，合并到当前仓库后不会自动迁移旧 run 或 batch。
