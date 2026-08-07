---
name: oz-exec
description: 当用户提到 oz exec，或要求执行一个活动 oz 提案时使用；用于读取 brief.md、acceptance.json 和 tests/ 硬合同，编写真实测试，按新意图更新过期历史测试并实现变更
---

# oz Exec

执行当前 `oz` 变更中的实现任务

## 入口差异

| 类型 | 执行阶段默认读取 | 硬合同 |
| --- | --- | --- |
| small | `proposal.md`、`brief.md`、`acceptance.json`、`tests/` | 仍必须先运行创建阶段契约测试，保留 acceptance 和 tests 硬合同 |
| standard | `proposal.md`、`brief.md`、`acceptance.json`、`tests/`，冲突时读 `design.md`、`spec.md` | 执行前必须验证验收合同覆盖提案成功标准；完整提案和验收合同必须保持一致 |

small 不降低测试要求；归档前仍要把长期行为沉淀到规格和规格测试。

## 外部实现与诊断技能（可选）

oz-exec 负责让 oz 提案的契约测试通过并生成真实证据。当实现需要 red-green 循环、交付前评审或顽固缺陷诊断时，可调用单独安装的 Matt Pocock 技能作为认知子程序；这些技能不由 `oz install` 安装，调用前确认已可用，不可用时回退到 oz-exec 内置流程。

- `/implement` + `/tdd`：在实现 spec/tickets 时优先使用 red-green-refactor 循环；本技能负责确保新增测试与 `acceptance.json` 对齐，且不得删除或弱化创建阶段写入的契约测试。
- `/code-review`：在移交给独立 QA 前，对当前 diff 做一次双轴 review（规范轴与规格轴）；把 review 结论作为自查 findings 处理，必要时先修复再进入 QA。
- `/prototype`：若实现中发现合同假设不成立，先做可丢弃原型验证再回改合同；如需调整 `spec.md`/`acceptance.json`，必须同步更新并记录原因，不得用原型代码直接冒充交付代码。
- `/diagnosing-bugs`：当遇到顽固 bug、性能退化或测试失败原因不明时调用；本技能负责把诊断结论和回归测试纳入当前提案范围，并按 oz-exec 的退出条件重新运行相关测试。
- `/codebase-design`：当需要 seam、interface、depth 等共享词汇来讨论模块设计时，作为参考词汇表查阅，不替代 oz-exec 的实现责任。

硬约束：不得用任何外部技能绕过、弱化或修改 `acceptance.json`/`tests/`；如需调整合同，先更新 `spec.md`、`acceptance.json` 和对应测试并记录原因，再继续实现。

调用时只须以斜杠命令唤起，不要在本文件中重复外部技能的内部流程；外部技能的具体步骤由各自的 `SKILL.md` 拥有。

## 流程

1. 确认当前提案目录已经提交；Oz 启动后封存 `delivery_base_head`，执行、自查、修复和测试阶段都不得创建或改写 commit。
2. 默认读取 `proposal.md`、`brief.md`、`acceptance.json` 和 `tests/`；`design.md`、`spec.md` 在实现路径涉及架构分歧或验收合同不足以覆盖意图时读取。禁止仅依据 `acceptance.json` 和 `tests/` 推断提案意图。
3. 先运行创建阶段契约测试，确认失败来自目标行为缺失，而不是测试语法、路径或环境问题。
4. **意图-合同对齐检查**：将 `acceptance.json` 的 `coverage`、`required_tests` 断言与 `proposal.md`（及 `spec.md`）的成功标准逐条对比。若验收合同仅以配置字符串、脚本存在、文件存在、HTTP 状态码、元素存在、日志包含子串、退出码为 0 等表面信号来断言本应是行为/语义/运行时变更的目标，必须视为“弱合同”——先补充行为级断言或更新 `spec.md`/`acceptance.json` 并说明原因，再继续实现。
5. 以实现提案意图为约束，实施最小可验证变更。
6. 运行相关测试和 `acceptance.json.required_tests` 中声明的命令。
7. 进入独立 QA 前，按仓库已有的显式入口运行提交前钩子，不得通过创建临时 commit 触发；吸收钩子改动后再次运行，确认不再修改文件。
8. 钩子稳定后重新运行受影响测试、全部 required tests 和 validation commands，确保快照绑定的是钩子处理后的最终内容。
9. 在执行器 Todo 或运行态中维护动态计划；交付时说明改动、验证和剩余风险。
10. 调用 `fix-code` 或 `fix-webapp` 时明确告知其处于 Oz 执行上下文：修复技能只修改、测试和整理证据，不自行提交；最终由 `oz-archive` 统一提交。

确认提案目录已提交到 git，防止后续操作误删：

```
git log --oneline -- docs/changes/<change>/
```

若未提交，必须在启动 Oz 工作流前先 `git add docs/changes/<change>/ && git commit -m "提案草稿: <change>"`；工作流启动后禁止补交、amend、rebase 或 squash。

默认先读取硬合同：

- `brief.md`
- `acceptance.json`
- `tests/` 中创建阶段已经写好的契约测试

`proposal.md` 是意图源，实现前必须完整读取。`design.md`、`spec.md` 在以下情况读取：
- 验收合同不足以覆盖 `proposal.md` 的成功标准；
- 历史测试与新意图冲突；
- 实现路径存在架构分歧。

读取后须用其成功标准校验 `acceptance.json` 与 `tests/` 的覆盖度，不能只提取“解决当前冲突所需”的片段。

实现时：

- 以当前提案和用户最新意图为准；实现前必须确认 `acceptance.json` 的断言能够覆盖 `proposal.md` 的成功标准，不能覆盖时须先补强合同
- 识别弱合同：若验收断言仅为以下类型，而提案目标是行为、语义或运行时保证，则合同不足，必须先补充行为级断言、更新 `acceptance.json` 和 `spec.md` 并说明原因：
  - 配置字符串、脚本存在性、文件存在性；
  - HTTP 状态码、元素存在、日志包含子串、退出码为 0；
  - 静态 fixture 或 mock 结果。
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
- 每次移交独立 QA 前，识别仓库声明的提交前钩子及其项目入口（例如项目脚本或 pre-commit、lefthook、husky 配置）；运行并吸收改动后至少再运行一次，第二次必须不再修改文件。若仍有改动，留在当前阶段排查，不得交给 QA。
- 结束前运行相关测试
- `test-results/` 只存放临时运行结果，禁止提交。稳定测试快照基线可按项目惯例跟踪；实际运行结果只能在最终通过后由 `oz-archive` 归集到 `tests/evidence/proposals/<change>/` 再跟踪
- 按 `delivery_report.scenarios` 实际走完用户路径并生成证据。Web 证据应让审核人员直接看懂操作与页面结果；命令行能力应保留真实输入、输出及业务状态变化
- 视频、截图、trace 必须是真实可打开的文件。echo/printf 字符串、退出码、HTTP 200、元素存在、测试通过字样或哈希不能冒充用户证据
- 修复前后对比必须来自同一入口、角色、数据和场景，并直观展示用户遇到的问题如何变成可用结果

## 长时 demo 执行要求

部分提案依赖长时间运行的 demo（如 `paper-orchestra-production-live`）生成真实证据。执行此类命令时必须遵守：

- 必须在 **foreground** 运行，让 demo 的 stdout/stderr 直接输出到当前终端/工作流 runner；
- **禁止**使用 `nohup ... &`、`setsid ... &`、Shell 后台作业，或将 stdout/stderr 重定向到文件后再 `sleep` 轮询；
- 不得通过 `sleep N && tail -f log` 等方式让 pi 节点自身进入长静默；
- 需要等待 demo 完成时，使用前台同步等待（如直接执行命令或 `wait`），让每一行进度日志都作为 go-dag pi 节点的心跳；
- 如果 demo 内部存在可能超过 5 分钟无输出的长耗时阶段，应确保脚本本身周期性打印进度/心跳，避免 go-dag 因单个阶段静默而终止 pi 节点。

## 退出条件

执行阶段只有在以下条件同时满足时才算完成：

- 当前提案范围内的实现已经完成，执行器 Todo 或运行态已收敛
- 创建阶段契约测试已经通过，失败记录已被新运行结果替代
- 相关根目录回归、端到端或包级测试已经运行，并记录命令和结果
- 没有删除、弱化、跳过或绕过 `acceptance.json` 与 `tests/`
- 如果更新了历史测试，已经说明它与新意图冲突的原因
- 仓库提交前钩子已经在最终 QA 前运行并确认幂等，钩子稳定后的 required tests 与 validation commands 已重新通过
- `proposal.md` 的每条成功标准都已被 `acceptance.json` 的测试断言覆盖，或有明确记录说明为何本次不覆盖以及由谁承担剩余风险
- 若提案要求行为、运行时或语义变更，实现必须包含对应代码或架构改动；仅靠配置字符串、脚本调整、文件增删过关的交付视为未完成
- `delivery_report` 声明的每个用户场景都已实际演示，证据文件真实可打开且普通审核人员能够理解

## 反偷懒检查

| 常见偷懒理由 | 处理方式 |
| --- | --- |
| “测试太慢，先不跑” | 结束前必须运行相关测试；不能把未运行测试交给 review/QA 猜 |
| “合同不方便，通过实现绕一下” | 先判断合同是否错误；确实错误时同步更新文档、JSON 和测试并说明原因 |
| “需要把动态计划持久化” | 只能使用执行器 Todo 或运行态；禁止创建或修改 `task.md`，也不得新增替代性的 Git 任务文件 |
| “历史测试失败是旧问题” | 只有证明和本次范围无关时才能列为剩余风险；与新意图冲突时要更新 |
| “提交时钩子自然会运行” | 最终 QA 后再首次运行会让已测试快照失效；必须在 QA 前显式运行、吸收改动并确认再次运行不再修改文件 |
| “契约测试只检查字符串/配置，我已经按要求改完了” | 必须把字符串断言映射到 `proposal.md` 成功标准；若无法映射，先补充行为级断言，再继续实现 |
| “现有回归测试已经通过，所以提案目标已实现” | 回归通过不等于新意图已实现；必须检查新增/变更的契约测试是否真实覆盖 `proposal.md` 新增成功标准 |
| “`acceptance.json` 没要求的行为我可以不做” | 执行阶段有责任发现并上报验收合同与提案意图的缺口，禁止利用合同漏洞 |

交付时说明实现内容、测试变更、历史测试更新原因、运行过的命令和剩余风险
