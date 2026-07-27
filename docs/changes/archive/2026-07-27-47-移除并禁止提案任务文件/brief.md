# 移除并禁止提案任务文件

## 用户问题

standard 提案当前要求在创建阶段写死 `task.md`，执行器又在执行过程中反复创建、修改和勾选该文件。这会把动态实现决策固化成 Git 文档，造成提案范围膨胀、校验重试和无意义审阅；执行器已经有 Todo 与持久运行态，不需要第二套任务清单。

## 交付目标

- standard 提案只保留 `brief.md`、`proposal.md`、`design.md`、`spec.md`、`acceptance.json` 和 `docs/changes/archive/2026-07-27-47-移除并禁止提案任务文件/tests/`，不再生成或要求 `task.md`。
- `oz validate` 接受不含 `task.md` 的 standard 提案；active 提案出现 `task.md` 时明确拒绝。
- 创建、执行和归档技能明令禁止创建或修改 `task.md`；动态计划只存在于执行器 Todo、state、stage artifact 与 acceptance 结果。
- 删除执行、状态和归档门禁对任务复选框的依赖；历史归档提案中的旧 `task.md` 只作为历史资料保留。

## 非目标

- 不删除 `docs/changes/archive/` 中历史提案的旧任务文件。
- 不规定具体执行器 Todo 工具的内部格式。
- 不用新的 Git 文件替代 `task.md`。

## 验收条目

1. 场景：standard 提案不含 `task.md` 仍能通过严格校验。
2. 场景：active 提案出现 `task.md` 时校验失败，且执行相关技能禁止创建或修改该文件。

## 执行入口

执行阶段默认读取本文件、`acceptance.json` 和 `docs/changes/archive/2026-07-27-47-移除并禁止提案任务文件/tests/test_no_task_file_contract.sh`。具体实现步骤由执行器动态规划，不得写入 Git 跟踪的任务文件。
