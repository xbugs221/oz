# 设计

## 类型判定

三种入口按行为和风险判定，不按代码行数机械判定。

```text
micro:
  不改变用户可感知行为
  不改变命令/API/状态语义
  不改变长期规格

small:
  改变行为或规格
  单一业务意图
  最多 2 个验收场景
  最多 2 个 required tests
  影响面小
  brief 可以讲清问题、范围、非目标和验收
  没有复杂设计分歧

standard:
  超过 small 的场景或测试上限
  跨模块或跨边界
  多场景、多角色或高风险
  涉及数据、权限、外部服务、迁移或回滚
  brief 写不清关键取舍
```

## 数量信号和升级触发器

不能把分类规则设计成“standard 至少 3 个测试、5 个任务”。这种硬门槛会诱导智能体拆碎任务、凑弱测试，反而降低提案质量。

分类应使用 small 上限和 standard 升级触发器：

| 信号 | 处理 |
| --- | --- |
| small 超过 2 个验收场景 | 升级 standard |
| small 超过 2 个 required tests | 升级 standard |
| small 需要记录关键技术取舍 | 升级 standard |
| small 影响多个命令、API、状态语义或长期规格 | 升级 standard |
| small 涉及权限、安全、数据迁移、外部服务或回滚 | 升级 standard |
| standard 只有 1 个测试或很少任务 | 必须说明命中的升级触发器，否则降级 small |
| 为了满足 standard 数量而拆碎任务或补弱测试 | 判定为无效提案设计 |

```text
small 上限：
  单一业务意图
  <= 2 个验收场景
  <= 2 个 required tests
  brief 一页内讲清

standard 触发：
  任一上限被突破
  或出现跨边界/高风险/设计取舍/多规格影响
```

## small 的文档合同

small 的 `brief.md` 必须覆盖原来长文档中对小改动真正有用的部分：

| 信息 | small 中的位置 |
| --- | --- |
| 为什么改 | `brief.md` 的问题 |
| 改什么 | `brief.md` 的范围 |
| 不改什么 | `brief.md` 的非目标 |
| 如何验收 | `brief.md` 的验收入口 + `acceptance.json` |
| 长期规格去向 | `brief.md` 或归档说明 |

`acceptance.json.coverage[].spec` 对 small 不再要求引用 `spec.md` 中的标题，而是允许引用 `brief.md` 中的验收场景，例如：

```text
brief：验收：空 tests 目录必须被 validate 拒绝
```

standard 继续使用现有 `需求：... / 场景：...` 引用方式。

## CLI 校验策略

`oz validate` 应当先识别提案类型：

| 条件 | 类型 |
| --- | --- |
| 有完整长文档 | standard |
| 只有 `brief.md`、`acceptance.json`、`docs/changes/archive/2026-07-09-43-支持三种提案入口/tests/` | small |
| 无 change 目录 | micro，不进入 validate |

small 必须校验：

- 目录名仍符合 `<编号>-<中文需求>`。
- `brief.md` 存在。
- `acceptance.json` 存在且结构合法。
- `docs/changes/archive/2026-07-09-43-支持三种提案入口/tests/` 存在且至少包含一个真实测试文件。
- `acceptance.json.required_tests` 引用真实存在的测试路径。
- `required_tests[].assertions` 至少包含业务级断言。

small 不校验：

- `proposal.md` 是否存在。
- `design.md` 是否存在。
- `spec.md` 是否存在。
- `task.md` 是否存在。
- `task.md` 是否包含复选框。

## 归档策略

small 不是临时便签。归档时仍必须阅读 `brief.md` 和 `docs/changes/archive/2026-07-09-43-支持三种提案入口/tests/`，把长期行为合并到 `docs/specs/`，把测试意图合并到 `tests/specs/`。

```text
small brief
    |
    v
archive 理解行为
    |
    +--> docs/specs/*.md
    |
    +--> tests/specs/*
```

## 风险

| 风险 | 缓解 |
| --- | --- |
| 所有复杂变更都偷懒写 small | skill 中定义升级条件，review/QA 可要求升级 standard |
| `brief.md` 变成压缩版六件套 | 限制 small 只写问题、范围、非目标、验收和规格去向 |
| small 的 coverage 没有 `spec.md` 锚点 | 允许引用 `brief.md` 验收句，同时归档后沉淀到主规格 |
