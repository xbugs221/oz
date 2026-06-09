# 合并 wo 执行器到 oz 仓库

## 背景

`wo` 项目依赖 `oz` 的提案目录、skill、`oz list/status/validate/archive` 命令和发布资产，但两个项目各自独立发布。当前 `wo` CI 会下载 GitHub latest `oz`，`wo update` 也分别检查和替换 `wo`、`oz`，因此执行器和规范工具容易处在不同版本批次。

更具体的问题是，`oz` README 和 `oz-create` skill 已把 `acceptance.json` 声明为提案产物，但当前 `oz validate/status` 还没有校验它；`wo` 则在 sealed run 创建时强依赖并严格校验 `acceptance.json`。这说明规范文本、规范 CLI 和执行器已经出现职责错位。

## 变更目标

- 将 `../wo` 的执行器源码、提示词模板、长期规格和真实业务测试合并进当前 `oz` 仓库。
- 在同一个 Go module 和同一个 Git tag 下构建、测试、发布 `oz` 与 `wo` 两个 CLI。
- 保留 `oz` 与 `wo` 的二进制边界，短期内继续让 `wo` 通过 `oz list/status/validate/archive` JSON 协议消费规范能力。
- 把 `acceptance.json` 提升为 `oz validate` 的正式提案契约，并复用当前 `wo` 已允许的严格 JSON 格式。
- 调整 CI、Release 和更新逻辑，避免从外部 latest `oz` 获取规范工具导致版本漂移。

## 非目标

- 不把 `wo` 状态机内联进 `oz` CLI；用户仍通过 `oz` 管提案规范，通过 `wo` 执行自动化工作流。
- 不在本变更中重写 sealed run、review、QA、fix、archive 主状态机。
- 不迁移用户状态目录中已有的未完成 `wo` run/batch；执行前应让用户自行完成或放弃旧仓库路径下的运行态。
- 不为 Web 页面或可视化界面新增能力。

## 验收重点

- 当前仓库可以从同一 checkout 构建 `oz` 和 `wo` 两个命令。
- `wo` 代码不再引用旧模块路径 `github.com/xbugs221/wo`。
- CI/Release 不再下载 `github.com/xbugs221/oz/releases/latest` 的外部资产来测试 `wo`。
- `oz validate` 接受当前 `wo` 允许的 `acceptance.json` 格式，并拒绝缺失或包含未知字段的验收合同。
- 原有 `oz` 与 `wo` Go 测试通过；关键 shell 业务测试能从合并后的仓库根目录运行。
