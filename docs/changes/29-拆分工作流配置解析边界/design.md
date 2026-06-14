# 设计：拆分工作流配置解析边界

## 目标结构

- `config.go`：公开配置类型、`LoadWorkflowConfig`、`DefaultWorkflowConfig`、`WorkflowConfig.StageOption`、配置路径和写配置入口。
- `config_schema.go`：`workflowConfigInput`、`stageOptionsInput`、YAML 解码、legacy root 检测和 `workflowConfigFromInput`。
- `config_profiles.go`：`BuiltInWorkflowProfiles`、`WorkflowProfileYAML`、profile 模板渲染、默认 prompt set。
- `config_parallel.go`：parallel/subagents/stages.before 输入转换、member 校验、allowed stages。
- `config_validation.go`：validation input、命令复制、limit 规范化。

## 关键取舍

本次只做文件级边界，不导出新 API。配置类型仍留在 `internal/app`，避免执行阶段为了拆包而扩大调用面。

## 风险

- `yaml.Decoder.KnownFields(true)` 是严格配置合同，不能丢失。
- legacy 字段错误信息需要包含字段名，shell 合同依赖这些诊断。
- profile 渲染需要继续注入 prompt 内容，不能生成空 prompt。
- `parallel: false` 必须继续关闭 `stages.before` 子代理图节点。

## 验证策略

合同测试先检查配置边界文件和 `config.go` 职责迁出，再运行 tree config、legacy config rejection、profile discovery、MADA profile 和 parallel config 既有业务合同脚本，产生日志证据。
