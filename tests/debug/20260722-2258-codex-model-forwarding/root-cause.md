# Codex 模型透传回归记录

## 场景

内置工作流配置新增固定模型后，需要证明配置中的 `model` 不仅被解析，还会成为 Codex 子进程的 `-m` 参数；仓库级阶段覆盖也应遵循同一链路。

## 证据与根因

- 既有 `codexExecArgs` 已将非空 `StageOptions.Model` 转换为 `-m <model>`。
- 原新增测试只断言 `WorkflowConfig.Stages` 中的字段值，没有覆盖最终命令参数，因此无法防止配置层与执行层之间的透传回归。
- 根因置信度：高。

## 修复方案

扩展内置配置测试，逐阶段检查生成的 Codex 参数；新增仓库配置自定义模型测试，验证用户值最终出现在 `-m` 参数中。

## 回归测试

```text
go test ./internal/app -run 'Test(BuiltInProfilesPinCodexModel|UserSelectedModelReachesCodexCommand)' -count=1
```

定向测试通过 2 项，`internal/app` 包级测试通过 132 项；有效配置合同
`test_remove_fixed_subagents_contract.sh` 通过，并覆盖真实生成配置中的固定模型。

## 剩余风险

测试覆盖参数构造边界，不启动真实 Codex 服务；真实 CLI 对具体模型名称的可用性仍由 Codex 自身校验。
