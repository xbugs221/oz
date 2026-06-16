# 设计：运行态清理与测试基建治理

## clean plan/apply

目标结构：

```text
CleanRuntimeStateWithOptions
  -> BuildCleanPlan
  -> ApplyCleanPlan
  -> formatCleanResult

oz flow clean --dry-run --json
  -> BuildCleanPlan
  -> JSON output
  -> no deletion
```

计划对象建议包含：

```text
CleanPlan
  repo
  runs[]
    id
    action delete | skip | protect
    reason
    path
  batches[]
    id
    action delete | skip | protect
    reason
    path
  sessions[]
    id
    action delete | protect
    provider
```

`ApplyCleanPlan` 只执行 plan 中标记为 delete 的动作，避免扫描和删除交织。

## 测试夹具

建议新增测试专用 helper 文件，例如 `workflow_fixture_test.go`：

```text
workflowFixture
  temp git repo
  installed fake oz/codex/pi tools
  write active change
  write acceptance contract
  save/load run state
  fake agent runner
```

目标不是隐藏业务断言，而是把重复的临时仓库和 fake runner 搭建逻辑集中起来。

## 风险

- clean plan 如果漏掉旧逻辑，可能改变删除范围。缓解方式是保留现有 clean Go 测试，并新增 dry-run/apply 一致性测试。
- 测试 fixture 提取如果过度抽象，会降低测试可读性。缓解方式是 fixture 只封装搭建，不封装业务断言。
