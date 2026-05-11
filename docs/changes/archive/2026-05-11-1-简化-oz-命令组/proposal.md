## 背景

当前 `oz` CLI 同时暴露 `init`、`plan`、`create`、`status`、`exec`、`validate`、`archive` 等命令。实际协作中，规划、创建和执行主要由智能体读取 skill 后完成，CLI 保留过多阶段入口会让用户误以为这些阶段仍应由命令行直接驱动。

本次变更把面向用户的日常命令收敛为两个高频辅助能力：查看活动提案，以及安装内置 skill。下游 `wo` 工具和内置 skill 依赖的机器接口继续保留，避免破坏自动化链路。

## 变更内容

- 面向用户的日常命令保留 `oz list [--json]` 和 `oz install [--global]` 两类。
- 新增 `oz l [--json]` 作为 `oz list [--json]` 的等价缩写。
- 新增 `oz i [--global]` 和 `oz i -g` 作为安装 skill 的等价缩写。
- 将现有 `oz init [--global]` 的安装能力迁移到 `oz install [--global]`。
- 保留 `oz status <change> [--json]`、`oz validate <change> [--json]`、`oz archive <change> --yes` 作为下游 `wo` 和 skill 使用的自动化接口。
- 顶层帮助和子命令帮助只展示保留命令及其缩写。
- README 同步区分日常命令和自动化接口，避免文档继续引导用户直接使用已移除的阶段入口。

## 能力范围

```text
oz
├── list | l [--json]
├── install | i [--global | -g]
└── automation
    ├── status <change> [--json]
    ├── validate <change> [--json]
    └── archive <change> --yes
```

`--version` 和 `-v` 作为 CLI 元信息保留，不视为业务命令。`init` 不作为隐藏兼容别名保留，`plan`、`create`、`exec` 等阶段入口应明确失败，改由智能体和 skill 驱动。

## 影响范围

- `cmd/oz/main.go` 的命令分发、帮助文本和安装命令命名。
- `cmd/oz/main_test.go` 中与旧命令面相关的测试。
- `README.md` 的命令列表和使用流程说明。
- 内置 skill 文件继续保留，且其中依赖的 `validate`、`archive` 自动化接口必须继续可用。
