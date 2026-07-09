# 规格

## 验收矩阵

| 需求 | 场景 | 测试 | 证据 | 风险 |
| --- | --- | --- | --- | --- |
| 三种提案入口 | 文档和 skill 说明 micro、small、standard | `proposal-entry-types-docs-contract` | 无 | 只验证入口规则文字和职责边界 |
| 三种提案入口 | 分类使用 small 上限和 standard 升级触发器 | `proposal-entry-types-docs-contract` | 无 | 不用硬性数量门槛 |
| small brief-only 校验 | `oz validate` 接受 small 最小目录且保留测试硬合同 | `small-brief-only-validate-contract` | 无 | 不覆盖交互式创建命令 |

### 需求：三种提案入口

系统必须把 `oz` 变更入口表达为 micro、small、standard 三种类型。

#### 场景：文档和 skill 说明 micro、small、standard

- **给定** 用户要判断一个改动是否需要提案
- **当** 用户阅读 README 和内置 oz skill
- **则** 文档必须说明 micro 使用 TDD 加 git commit
- **并且** 文档必须说明 small 使用 `brief.md`、`acceptance.json` 和 `tests/`
- **并且** 文档必须说明 standard 使用完整提案文档
- **并且** 文档必须说明 small 仍保留真实测试和长期规格归档要求

#### 场景：分类使用 small 上限和 standard 升级触发器

- **给定** 用户或智能体不确定一个变更应归类为 small 还是 standard
- **当** 用户阅读 README、长期规格和内置 oz skill
- **则** 文档必须说明 small 的上限是单一业务意图、最多 2 个验收场景、最多 2 个 required tests
- **并且** 文档必须说明超过 small 上限、跨模块、高风险、权限、安全、数据迁移、外部服务、回滚或设计取舍复杂时必须升级 standard
- **并且** 文档不得要求 standard 通过硬凑测试数量或任务数量来成立
- **并且** standard 如果只有少量测试或任务，必须说明为什么不能降级为 small

### 需求：small brief-only 校验

系统必须允许 small 提案省略长文档，但不得省略验收硬合同。

#### 场景：`oz validate` 接受 small 最小目录且保留测试硬合同

- **给定** 一个 small active change 只有 `brief.md`、`acceptance.json` 和 `tests/`
- **当** 用户运行 `oz validate <change> --json`
- **则** 校验必须通过
- **并且** `acceptance.json.required_tests` 必须引用真实存在的测试文件
- **并且** 空 `tests/` 或只有说明文档的 `tests/` 仍必须失败
- **并且** standard 提案的完整文档校验不得被移除
