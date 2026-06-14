# 提案：拆分 status view 渲染边界

## 背景

`status_view.go` 已经成为 status/watch 变更的主要维护热点。它混合了数据模型、业务状态推导、终端渲染和工具函数，导致开发者很难判断某次修改是否只影响人类输出，还是会影响 JSON observability。

## 目标

- 将 status view 构建逻辑移动到清晰的模型文件。
- 将阶段耗时和 workflow wall time 计算独立出来。
- 将紧凑终端渲染、列宽、中文宽度计算独立出来。
- 将 stale running run 的显示判断独立出来。
- 保持现有 status/watch JSON 和人类输出合同不变。

## 非目标

- 不修改 status/watch 用户可见格式。
- 不调整 `State` 持久化字段。
- 不新增新的状态枚举。
