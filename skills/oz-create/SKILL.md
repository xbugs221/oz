---
name: oz-create
description: 当用户提到 oz create，或要求创建 oz 变更提案时使用；用于创建 docs/changes/<编号>-<中文提案名>/ 及 proposal.md、design.md、spec.md、task.md、acceptance.json、tests/
---

# oz Create

创建 `oz` 变更提案并生成所有产物

- `proposal.md`：说明做什么和为什么
- `design.md`：说明关键技术决策、取舍和风险
- `spec.md`：使用中文 `### 需求：` 和 `#### 场景：` 描述验收行为，说明 `tests/` 中每个测试文件覆盖的核心业务契约、真实数据来源、入口路径、断言和剩余风险
- `task.md`：拆分实现步骤和验证条件，第一组任务必须是先运行创建阶段写好的契约测试；如果功能尚未实现，预期应因目标行为缺失而失败，不得因测试语法、路径或环境配置错误失败
- `acceptance.json`：结构化验收合同，必须复用 `tests/` 中的契约测试，并补齐根目录端到端/回归测试、截图、trace、network、console、runtime log 等 QA 证据要求
- `tests/`：必须包含真实项目测试代码，用来表达本次变更的核心契约；不得为空，不写测试说明文档或占位文件，但是测试文件内部要多用中文写批注，确保非专业软件工程师也能看懂

创建阶段负责定义核心契约测试和完整验收合同，执行阶段负责让这些测试通过。不得把核心行为标准推迟给执行器自行制定。

- 覆盖用户可感知行为：页面可见结果、CLI 输出、API 契约、状态变化、持久化结果或权限边界
- 使用真实业务样例、真实入口和真实调用链；Web app 参照 debug 技能的端到端规则，不能跳过登录、权限、真实导航、真实 API 或真实数据库
- 不得 mock API、mock 数据库、伪造认证、硬编码成功结果、只断言 HTTP 200，除非用户明确要求且在 `design.md` 写清原因和风险
- 不得用静态 fixture 假装真实业务数据；如果必须使用仓库既有 fixture，必须说明它代表的真实业务样例和字段含义
- 如果缺少真实账号、真实样例数据、外部服务权限或可运行环境，先向用户澄清；不要创建会让执行器自行编造数据的提案
- 如果当前仓库完全没有可用测试框架，也必须在 `tests/` 写出最小可运行的项目测试入口；引入新框架前先确认这是最小代价

`acceptance.json` 使用严格 JSON，形如：

```json
{
  "summary": "本次变更的验收重点",
  "required_tests": [
    {
      "id": "contract-main-flow",
      "source": "change_contract",
      "path": "docs/changes/12-示例/tests/main-flow.acceptance.test.ts",
      "command": "pnpm exec tsx --test docs/changes/12-示例/tests/main-flow.acceptance.test.ts",
      "purpose": "证明 proposal/spec 中的主业务路径契约"
    }
  ],
  "required_evidence": [
    {
      "id": "screenshot-after-reload",
      "kind": "screenshot",
      "path": "test-results/main-flow/after-reload.png",
      "purpose": "证明刷新后状态恢复"
    }
  ]
}
```

`required_tests[].source` 仅使用 `change_contract`、`root_e2e`、`existing_regression`、`new_regression`。`required_evidence[].kind` 仅使用 `screenshot`、`trace`、`network`、`console`、`runtime_log`、`state_snapshot`、`other`。

如果暂时无法确定测试策略、真实数据来源、用户可感知断言或 QA 证据要求，先和用户澄清；不要创建缺少契约测试或 `acceptance.json` 的提案。

执行阶段可以新增单元测试、回归测试、端到端测试或边界测试，但不得删除、弱化或绕过创建阶段写入的契约测试和 `acceptance.json`。只有用户明确变更意图时，才能同步更新 `spec.md`、`design.md`、`task.md`、`acceptance.json` 和 `tests/`，并记录原因。

目录名为 `docs/changes/<number>-<change-name>/`，不要写日期前缀。创建目录前先运行 `oz create` 获取 `<number>`:

- `<number>` 使用 `oz create` 标准输出中的整数
- 不要通过展开 `docs/changes/` 或 `docs/changes/archive/` 的目录清单来计算编号；历史提案很多时，这会浪费智能体上下文
- `<change-name>` 必须是中文需求描述，可以混用英文单词、数字和连字符，但必须包含中文汉字，不能全英文

然后运行 `oz validate <change> --json` 检验，确认 `tests/` 目录存在、包含测试代码且没有占位内容；同时确认 `acceptance.json` 是合法 JSON，且其中的 `required_tests` / `required_evidence` 与文档和测试路径一致。

完成后，立刻 commit 这部分提案，message格式："<number>提案: <change-name>"
