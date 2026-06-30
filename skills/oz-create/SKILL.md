---
name: oz-create
description: 当用户提到 oz create，或要求创建 oz 变更提案时使用；用于创建 docs/changes/<编号>-<中文提案名>/ 及 brief.md、proposal.md、design.md、spec.md、task.md、acceptance.json、tests/
---

# oz Create

创建 `oz` 变更提案并生成所有产物。创建阶段不是写愿景文档，而是建立可执行的交付合同；`brief.md`、`proposal.md`、`spec.md`、`acceptance.json` 和 `tests/` 的承诺必须等强度对齐。

## 流程

1. 先运行 `oz create` 获取下一个编号，只把输出整数作为 `<number>`。
2. 根据规划结果创建 `docs/changes/<number>-<change-name>/`。
3. 先列出 `spec.md` 场景到 `required_tests`、`required_evidence` 的验收矩阵。
4. 再写 `brief.md`、`proposal.md`、`design.md`、`spec.md`、`task.md`、`acceptance.json` 和 `tests/`。
5. 运行 `oz validate <change> --json`，并人工核对文档、测试和 JSON 合同是否一致。
6. 创建阶段完成后提交提案产物，避免执行阶段误删或混入无关改动。

## 产物合同

- `brief.md`：用短文说明用户问题、交付目标、非目标、验收入口和执行阶段默认上下文；让执行器无需先读长文档也能理解本次变更
- `proposal.md`：说明做什么和为什么
- `design.md`：说明关键技术决策、取舍和风险
- `spec.md`：使用中文 `### 需求：` 和 `#### 场景：` 描述验收行为；每个场景都必须说明对应 `tests/` 文件、真实数据来源、入口路径、关键断言和剩余风险
- `task.md`：拆分实现步骤和验证条件，第一组任务必须是先运行创建阶段写好的契约测试；如果功能尚未实现，预期应因目标行为缺失而失败，不得因测试语法、路径或环境配置错误失败
- `acceptance.json`：结构化验收合同，必须逐条覆盖 `spec.md` 中的需求和场景，复用 `tests/` 中的契约测试，并补齐根目录端到端/回归测试、截图、trace、network、console、runtime log 等 QA 证据要求
- `tests/`：必须包含真实项目测试代码，用来表达本次变更的核心契约；不得为空，不写测试说明文档或占位文件，但是测试文件内部要多用中文写批注，确保非专业软件工程师也能看懂

创建阶段负责定义简报、核心契约测试和完整验收合同，执行阶段负责让这些测试通过。不得把核心行为标准推迟给执行器自行制定，也不得让文档承诺高于测试能证明的行为。

- 先列出 `spec.md` 中每个 `#### 场景：` 的验收矩阵，再写正文；每个场景至少对应一个 `required_tests`，关键用户路径还必须对应 `required_evidence`
- `proposal.md` 和 `spec.md` 不得写入没有测试或证据覆盖的承诺；如果暂时无法验证，只能降级为风险、非目标或开放问题
- 覆盖用户可感知行为：页面可见结果、CLI 输出、API 契约、状态变化、持久化结果或权限边界
- 使用真实业务样例、真实入口和真实调用链；Web app 参照 debug 技能的端到端规则，不能跳过登录、权限、真实导航、真实 API 或真实数据库
- 不得 mock API、mock 数据库、伪造认证、硬编码成功结果、只断言 HTTP 200、只检查元素存在、只跑组件浅层渲染，除非用户明确要求且在 `design.md` 写清原因和风险
- 不得用静态 fixture 假装真实业务数据；如果必须使用仓库既有 fixture，必须说明它代表的真实业务样例和字段含义
- 如果缺少真实账号、真实样例数据、外部服务权限或可运行环境，先向用户澄清；不要创建会让执行器自行编造数据的提案
- 如果当前仓库完全没有可用测试框架，也必须在 `tests/` 写出最小可运行的项目测试入口；引入新框架前先确认这是最小代价
- 契约测试脚本必须包含业务级断言，断言对象应是输出内容、数据库记录、API 响应字段、权限拒绝、持久化状态、审计日志或用户可见流程结果；不能只证明测试文件能运行
- 测试结果、截图、trace、runtime log 等 evidence artifact 是运行产物，默认写入 `test-results/` 并被 git 忽略；仓库只跟踪测试代码、验收合同和必要 fixture。不得在契约测试中用 `git ls-files --error-unmatch` 要求 `test-results` 被跟踪，不得通过修改 `.gitignore` 或 `git add -f` 让 `test-results` 成为交付物。若某份证据必须长期版本化，应放入 `docs/changes/<change>/evidence/`，不要放入 `test-results/`。
- 创建完成前要自查一次：如果实现者只做最小表面实现也能通过测试，必须继续加断言或收窄文档承诺

`acceptance.json` 使用严格 JSON，形如：

```json
{
  "summary": "本次变更的验收重点",
  "coverage": [
    {
      "spec": "需求：示例能力 / 场景：主业务路径",
      "tests": ["contract-main-flow"],
      "evidence": ["screenshot-after-reload"],
      "risk": "未覆盖的边界或外部依赖"
    }
  ],
  "required_tests": [
    {
      "id": "contract-main-flow",
      "source": "change_contract",
      "path": "docs/changes/12-示例/tests/main-flow.acceptance.test.ts",
      "command": "pnpm exec tsx --test docs/changes/12-示例/tests/main-flow.acceptance.test.ts",
      "purpose": "证明 proposal/spec 中的主业务路径契约",
      "assertions": [
        "使用真实入口创建一条带关键字段的业务记录",
        "刷新或重新查询后仍能看到持久化结果",
        "缺少权限时返回明确拒绝而不是静默成功"
      ],
      "expected_initial_failure": "功能未实现时应失败于目标业务行为缺失"
    }
  ],
  "required_evidence": [
    {
      "id": "screenshot-after-reload",
      "kind": "screenshot",
      "path": "test-results/main-flow/after-reload.png",
      "purpose": "证明刷新后状态恢复；该文件是本地运行产物，不进入 git"
    }
  ]
}
```

`coverage[].spec` 必须引用 `spec.md` 中真实存在的需求和场景；`coverage[].tests` 必须引用 `required_tests[].id`；`coverage[].evidence` 必须引用 `required_evidence[].id`，没有证据时写空数组并在 `risk` 解释。`required_tests[].source` 仅使用 `change_contract`、`root_e2e`、`existing_regression`、`new_regression`。`required_tests[].assertions` 至少列出一个业务级断言。`required_evidence[].kind` 仅使用 `screenshot`、`trace`、`network`、`console`、`runtime_log`、`state_snapshot`、`other`。

`required_evidence` 表示 QA/执行阶段可复核的运行证据，不表示该产物必须进入版本控制。`test-results/` 下的 evidence 只要求能由 `required_tests[].command` 或明确记录的 QA 命令重新生成并复核。

如果暂时无法确定测试策略、真实数据来源、用户可感知断言或 QA 证据要求，先和用户澄清；不要创建缺少契约测试或 `acceptance.json` 的提案。

执行阶段可以新增单元测试、回归测试、端到端测试或边界测试，但不得删除、弱化或绕过创建阶段写入的契约测试和 `acceptance.json`。只有用户明确变更意图时，才能同步更新 `spec.md`、`design.md`、`task.md`、`acceptance.json` 和 `tests/`，并记录原因。

## 退出条件

创建阶段只有在以下条件同时满足时才算完成：

- 目录名为 `docs/changes/<number>-<change-name>/`，不要写日期前缀。创建目录前先运行 `oz create` 获取 `<number>`:

- `<number>` 使用 `oz create` 标准输出中的整数
- 不要通过展开 `docs/changes/` 或 `docs/changes/archive/` 的目录清单来计算编号；历史提案很多时，这会浪费智能体上下文
- `<change-name>` 必须是中文需求描述，可以混用英文单词、数字和连字符，但必须包含中文汉字，不能全英文。尽量写成动宾短语格式，让人一眼能看明白提案的意图

- 运行 `oz validate <change> --json` 检验，确认 `brief.md` 存在且能支撑执行阶段默认上下文，`tests/` 目录存在、包含测试代码且没有占位内容
- 人工核对 `acceptance.json` 是合法 JSON，且其中的 `coverage` / `required_tests` / `required_evidence` 与文档、测试路径和测试脚本内部断言一致
- 完成后，立刻 commit 这部分提案，message格式："<number>提案: <change-name>"

## 反偷懒检查

| 常见偷懒理由 | 处理方式 |
| --- | --- |
| “先写文档，测试执行阶段补” | 创建阶段必须写真实契约测试和 `acceptance.json`，否则执行阶段没有硬合同 |
| “只要有 `spec.md` 就够了” | `spec.md` 中每个场景都必须被测试或 evidence 覆盖 |
| “test-results 也提交进去更稳” | `test-results/` 是本地运行产物，默认不进 git；需要长期版本化时放入 change 的 `evidence/` |
| “真实数据不好准备，先 mock” | 缺真实账号、样例数据或权限时先问用户，不让执行器编造 |
