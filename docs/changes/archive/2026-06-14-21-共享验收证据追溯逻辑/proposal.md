# 文件目的

本文件说明共享验收 evidence producer 追溯逻辑的动机和交付内容。

## 背景

`acceptance.json` 是 `oz` 提案和 `wo` 执行工作流之间的硬合同。证据 producer 追溯用于保证 required evidence 不是只写在文档里的愿望，而是能被 required test 稳定产出。

## 问题

当前相同规则散落在两个入口：

- `oz validate` 用于创建和归档前校验。
- `wo` execution 后 acceptance preflight 用于阻断不可验收的运行。

重复实现会增加长期维护风险。

## 变更

- 在 `internal/acceptance` 新增共享 producer 追溯函数。
- `cmd/oz` 删除本地重复 helper。
- `internal/app` 删除本地重复 helper，只保留 workflow 状态转换和错误文案。
- 增加共享包单元测试覆盖元数据命中、测试源码命中、同目录 shell wrapper 命中和无 producer 失败。

## 稳定性原则

本提案必须保持现有合法 acceptance 合同继续合法，现有非法合同继续非法。重构只改变代码归属，不改变业务规则强度。
