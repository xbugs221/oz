# 提案：拆分工作流配置解析边界

## 背景

`config.go` 同时负责公开配置类型、默认配置、profile 渲染、YAML schema、legacy 字段拒绝、parallel 展开、validation 配置和 prompt 注入。配置面是用户直接编辑的合同，逻辑集中在一个大文件会让后续新增字段或移除兼容逻辑变得难审查。

## 变更

- 保留 `config.go` 中公开类型和 `LoadWorkflowConfig`、`DefaultWorkflowConfig` 等入口。
- 新增 `config_schema.go`，承载 YAML input 类型、KnownFields 解码和 legacy root 拒绝。
- 新增 `config_profiles.go`，承载内置 profile 注册、profile YAML 渲染和 prompt 注入。
- 新增 `config_parallel.go`，承载 `parallel`、`subagents` 和 `stages.<stage>.before` 展开与校验。
- 新增 `config_validation.go`，承载 validation 命令和 retry limit 解析。

## 为什么

配置解析有多个变化原因：用户 schema、内置 profile、并行子代理拓扑、验证命令。拆开后可以针对每类配置变更写小范围测试和审查，不必在一个近千行文件里定位。

## 非目标

- 不拆 package。
- 不改变默认 profile 内容。
- 不改变旧字段拒绝策略。
