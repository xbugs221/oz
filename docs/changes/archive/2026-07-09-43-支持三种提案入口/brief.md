# 支持三种提案入口

## 问题

当前 `oz` 只有一种完整提案形态。它适合中大型变更，但小型行为改动如果也强制写 `proposal.md`、`design.md`、`spec.md` 和 `task.md`，会把流程变成形式主义。反过来，如果小型改动只用 TDD 加 git commit，又容易绕过全局规格，长期形成细节矛盾。

## 目标

把变更入口拆成三类：

| 类型 | 适用场景 | 产物 |
| --- | --- | --- |
| micro | 不改变用户可感知行为、命令契约、状态语义或长期规格的纯实现修复 | TDD + git commit，不建 change 目录 |
| small | 单一业务意图、最多 2 个验收场景、最多 2 个 required tests，且无复杂设计分歧 | `brief.md` + `acceptance.json` + `tests/` |
| standard | 中大型、高风险、跨模块或多场景变更 | 现有完整提案六件套 + `acceptance.json` + `tests/` |

```text
是否改变行为/规格？
        |
        +-- 否：micro
        |
        +-- 是，但范围小：small
        |
        +-- 是，且跨模块/高风险/多场景：standard
```

数量只作为分类信号，不作为凑数门槛。standard 如果只有 1 个测试或很少任务，必须解释为什么不能降级为 small；small 如果超过 2 个验收场景或 2 个 required tests，应升级为 standard。

## 范围

- 更新 `README.md`、内置 skill 和长期规格，说明 micro/small/standard 三种入口。
- 更新 `oz validate`，允许 small 提案只包含 `brief.md`、`acceptance.json` 和 `tests/`。
- 保持 small 的测试硬合同：`acceptance.json` 和 `tests/` 仍然必填，测试必须是真实测试代码。
- 保持 standard 的完整文档要求，避免复杂变更降级成 brief-only。
- 明确 small 上限和 standard 升级触发器，避免用硬性测试数量或任务数量诱导凑数。
- 归档时 small 也必须把长期行为合并进 `docs/specs/`，测试意图合并进 `tests/specs/`。

## 非目标

- 不为 micro 创建 change 目录。
- 不删除现有 standard 提案能力。
- 不放宽 `acceptance.json.required_tests[].assertions` 的业务断言要求。
- 不要求自动判断所有变更类型；智能体和用户仍需要按规则选择入口。

## 验收入口

- `bash docs/changes/43-支持三种提案入口/tests/test_proposal_entry_types_docs_contract.sh`
- `bash docs/changes/43-支持三种提案入口/tests/test_small_brief_only_validate_contract.sh`

执行阶段先运行以上测试。当前代码下第二个测试应因 `oz validate` 仍要求完整六件套而失败；实现完成后必须通过。

## 执行阶段默认上下文

优先读取本文件、`acceptance.json` 和 `tests/`。只有需要理解完整设计取舍时再读取 `proposal.md`、`design.md`、`spec.md` 和 `task.md`。
