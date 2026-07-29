---
name: oz-exec
description: 当用户提到 oz exec，或要求执行一个活动 oz 提案时使用；用于读取 brief.md、acceptance.json 和 tests/ 硬合同，编写真实测试，按新意图更新过期历史测试并实现变更
---

# oz Exec

执行当前 `oz` 变更中的实现任务

## 入口差异

| 类型 | 执行阶段默认读取 | 硬合同 |
| --- | --- | --- |
| small | `brief.md`、`acceptance.json`、`tests/`，即 brief-only | 仍必须先运行创建阶段契约测试，保留 acceptance 和 tests 硬合同 |
| standard | `brief.md`、`acceptance.json`、`tests/`，必要时读取 `proposal.md`、`design.md`、`spec.md` | 完整提案和验收合同都必须保持一致 |

small 不降低测试要求；归档前仍要把长期行为沉淀到规格和规格测试。

## 流程

1. 确认当前提案目录已经提交；Oz 启动后封存 `delivery_base_head`，执行、自查、修复和测试阶段都不得创建或改写 commit。
2. 默认只读取 `brief.md`、`acceptance.json` 和 `tests/`，先抓硬合同。
3. 先运行创建阶段契约测试，确认失败来自目标行为缺失，而不是测试语法、路径或环境问题。
4. 实现最小可验证变更，并按需读取长文档解决冲突。
5. 运行相关测试和 `acceptance.json.required_tests` 中声明的命令。
6. 在执行器 Todo 或运行态中维护动态计划；交付时说明改动、验证和剩余风险。
7. 调用 `fix-code` 或 `fix-webapp` 时明确告知其处于 Oz 执行上下文：修复技能只修改、测试和整理证据，不自行提交；最终由 `oz-archive` 统一提交。

确认提案目录已提交到 git，防止后续操作误删：

```
git log --oneline -- docs/changes/<change>/
```

若未提交，必须在启动 Oz 工作流前先 `git add docs/changes/<change>/ && git commit -m "提案草稿: <change>"`；工作流启动后禁止补交、amend、rebase 或 squash。

默认先读取硬合同：

- `brief.md`
- `acceptance.json`
- `tests/` 中创建阶段已经写好的契约测试

`proposal.md`、`design.md`、`spec.md` 只在验收合同冲突、用户最新意图冲突、历史测试需要更新或实现路径存在架构分歧时按需读取；读取后只提取解决当前冲突所需的信息。

实现时：

- 以当前提案和用户最新意图为准
- 审查 `tests/specs/` 和根目录 `tests/` 中与本次变更相关的历史测试；`tests/specs/` 按业务能力组织，不按提案编号机械分组
- 如果历史测试与新意图冲突，更新测试代码，并在 `design.md` 或交付说明中记录原因
- 先运行创建阶段写入 `docs/changes/<change>/tests/` 的契约测试；如果功能尚未实现，失败原因应指向目标行为缺失
- 不得删除、弱化、跳过或改写创建阶段的契约测试或 `acceptance.json` 来让实现过关
- 如果合同要求直接跟踪 `test-results/` 下的截图、trace 或 runtime log，应判定为验收合同错误并同步修正；运行结果只有经 `oz-archive` 归集到 `tests/evidence/proposals/<change>/` 后才能跟踪，不得用 `git add -f` 绕过
- 如用户最新意图明确改变验收标准，必须先同步更新 `spec.md`、`design.md`、`acceptance.json` 和对应测试，并写明变更原因，再继续实现
- 可以新增补充测试，但新增测试必须是真实项目测试代码；契约补充写入 `docs/changes/<change>/tests/`，端到端/回归验收可按项目惯例写入根目录测试集，并同步更新 `acceptance.json`
- 不得 mock API、mock 数据库、伪造认证、硬编码成功结果或只断言 HTTP 200，除非用户明确要求且已在提案文档记录风险
- 不在 `tests/` 写占位文档
- 禁止创建或修改 `task.md`；不得用其他 Git 跟踪文件保存动态实现步骤，计划只进入 Todo 或运行态
- 执行、自查、修复和测试阶段禁止提交；归档阶段会把 `delivery_base_head` 之后的全部交付内容一次性提交
- 结束前运行相关测试
- `test-results/` 只存放临时运行结果，禁止提交。稳定测试快照基线可按项目惯例跟踪；实际运行结果只能在最终通过后由 `oz-archive` 归集到 `tests/evidence/proposals/<change>/` 再跟踪
- 按 `delivery_report.scenarios` 实际走完用户路径并生成证据。Web 证据应让审核人员直接看懂操作与页面结果；命令行能力应保留真实输入、输出及业务状态变化
- 视频、截图、trace 必须是真实可打开的文件。echo/printf 字符串、退出码、HTTP 200、元素存在、测试通过字样或哈希不能冒充用户证据
- 修复前后对比必须来自同一入口、角色、数据和场景，并直观展示用户遇到的问题如何变成可用结果

## 退出条件

执行阶段只有在以下条件同时满足时才算完成：

- 当前提案范围内的实现已经完成，执行器 Todo 或运行态已收敛
- 创建阶段契约测试已经通过，失败记录已被新运行结果替代
- 相关根目录回归、端到端或包级测试已经运行，并记录命令和结果
- 没有删除、弱化、跳过或绕过 `acceptance.json` 与 `tests/`
- 如果更新了历史测试，已经说明它与新意图冲突的原因
- `delivery_report` 声明的每个用户场景都已实际演示，证据文件真实可打开且普通审核人员能够理解

## 反偷懒检查

| 常见偷懒理由 | 处理方式 |
| --- | --- |
| “测试太慢，先不跑” | 结束前必须运行相关测试；不能把未运行测试交给 review/QA 猜 |
| “合同不方便，通过实现绕一下” | 先判断合同是否错误；确实错误时同步更新文档、JSON 和测试并说明原因 |
| “需要把动态计划持久化” | 只能使用执行器 Todo 或运行态；禁止创建或修改 `task.md`，也不得新增替代性的 Git 任务文件 |
| “历史测试失败是旧问题” | 只有证明和本次范围无关时才能列为剩余风险；与新意图冲突时要更新 |

交付时说明实现内容、测试变更、历史测试更新原因、运行过的命令和剩余风险
