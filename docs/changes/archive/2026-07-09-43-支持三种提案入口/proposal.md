# 支持三种提案入口

## 背景

`oz` 的核心价值是让需求、测试和长期规格可复盘。现有单一完整提案模型对中大型变更有效，但对小型变更成本偏高。

小型行为改动常见特征是：

- 只改一两个文件或少量函数。
- 仍会改变用户可见行为、命令输出、校验规则或长期规格。
- 不需要独立设计文档就能讲清边界。
- 需要一条简短记录解释为什么改、改到什么边界、测试证明什么。

如果这类改动直接 TDD 加提交，局部实现可能和全局规格脱节；如果强制完整提案，则文档成本超过改动成本。

## 提议

引入三种入口：

| 入口 | 决策标准 | 交付方式 |
| --- | --- | --- |
| micro | 不改变行为和规格，只是内部修复、重命名、格式或局部重构 | TDD + git commit |
| small | 改变行为或规格，但只有单一业务意图、最多 2 个验收场景、最多 2 个 required tests，且无复杂设计分歧 | brief-only change |
| standard | 跨模块、高风险、多场景、设计取舍复杂、验收口径有争议，或超过 small 上限 | 完整 change |

small 提案的最小目录：

```text
docs/changes/<N-中文需求>/
├── brief.md
├── acceptance.json
└── tests/
    └── <真实测试代码>
```

standard 提案保持现有目录：

```text
docs/changes/<N-中文需求>/
├── brief.md
├── proposal.md
├── design.md
├── spec.md
├── task.md
├── acceptance.json
└── tests/
```

数量规则只用于降低分类歧义，不用于凑数。不能规定 standard 必须不少于 3 个测试或 5 个任务；如果一个 standard 自然只有少量测试或任务，提案必须说明它命中哪类升级触发器。

## 成功标准

- 用户和智能体能在创建前判断 micro/small/standard。
- small 不再要求 `proposal.md`、`design.md`、`spec.md`、`task.md`。
- small 的上限和 standard 的升级触发器清楚可查，避免按文件数量或任务数量机械分类。
- small 仍必须有真实测试和 `acceptance.json`，不能退化成纯说明文档。
- `oz validate` 能区分 small 与 standard，不因 small 缺少长文档而失败。
- 归档逻辑仍要求把已验证行为沉淀到长期规格和长期规格测试。
