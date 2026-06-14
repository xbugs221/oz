# 提案：彻底合并 wo 为 oz flow

## 背景

当前仓库已经把 `oz` 与 `wo` 放在同一个 Go module、同一套源码和同一批发布流程里，但仍然保留两个 CLI 入口：`cmd/oz` 负责提案规范、技能安装、校验和归档，`cmd/wo` 负责自动化工作流执行。

这对个人工具而言已经超过必要复杂度：安装、CI、Release、README、规格和测试都要同时维护两个命令名。继续保留兼容别名会让代码库长期背负旧命名和双入口心智负担。

## 目标

本次变更做破坏性清理，把工作流能力作为 `oz flow` 命令组合并到唯一 `oz` CLI 中。最终仓库只发布一个 `oz` 二进制，用户使用：

- `oz flow status`
- `oz flow watch`
- `oz flow run`
- `oz flow config`
- `oz flow clean`
- `oz flow restart`

所有旧 `wo` 命令、兼容入口、旧提示和双二进制发布合同都应删除。

## 非目标

- 不提供 `wo` 到 `oz flow` 的兼容 shim。
- 不迁移已有本地 `wo` 状态目录或 `wo.yaml`。
- 不新增工作流功能。
- 不重构 DAG 执行器内部算法。

## 成功标准

- `go build ./cmd/oz` 可以生成唯一 CLI。
- 仓库不存在 `cmd/wo`。
- CI/Release 不再构建或引用 `cmd/wo`。
- `oz flow --help`、`oz flow status`、`oz flow watch` 等工作流入口由真实 CLI 分发，不是外部 shell 包装。
- 活跃源码、README、CI、长期规格测试和模板命名不再保留 `wo` 产品面。
