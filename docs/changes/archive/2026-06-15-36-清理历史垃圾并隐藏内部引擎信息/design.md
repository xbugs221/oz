# 设计

## 边界划分

本次清理分为三层：

```text
用户可见面
  README / docs/specs / prompts-template / profiles-template
  oz flow help / config / graph / status / watch / run error
  约束：不得出现 go-dag、Dagu、wo 旧产品面

活跃维护面
  cmd / internal / docs/changes/archive/2026-06-15-36-清理历史垃圾并隐藏内部引擎信息/tests/specs / .github/workflows
  约束：不得保留旧 cmd/wo、wo.yaml、.wo、/wo/repos、legacy-agent/opencode 合同

历史归档面
  docs/changes/archive/**
  约束：默认不清理，作为历史审计记录保留
```

`go-dag` 可以继续作为内部包、文件或函数命名存在，但不能被输出给非开发用户，也不能写进用户配置、规格说明或当前业务测试的可见断言。

## 测试层收敛

根目录 `docs/changes/archive/2026-06-15-36-清理历史垃圾并隐藏内部引擎信息/tests/2026-*` 是历史迁移层，不再代表当前业务合同。执行阶段应逐个判断：

- 已被 `docs/changes/archive/2026-06-15-36-清理历史垃圾并隐藏内部引擎信息/tests/specs` 或 Go 测试覆盖的脚本直接删除。
- 仍有业务价值但尚未覆盖的场景，迁移为当前 `oz flow` specs 测试或 Go 测试。
- 删除后更新 `test_root_test_layout_contract.sh`，避免未来重新引入 dated legacy shell 测试。

## 旧输入拒绝合同

部分旧格式仍需要被拒绝，例如旧配置根节点或旧字段。保留这类测试时必须满足：

- 测试名称和说明写清“拒绝旧输入”。
- 旧字段只出现在临时 fixture 或 heredoc 中。
- 用户文档不再把旧格式当成迁移路径或兼容能力。

## 风险

- 大量删除历史 shell 测试可能丢失边界覆盖。缓解方式是先用 `rg` 对每个脚本归类，必要场景迁移到当前测试层。
- 隐藏 `go-dag` 可能影响调试输出。开发者调试信息应放在内部日志或源码注释中，不进入默认 CLI stdout/stderr。
- 扫描测试容易误杀历史归档。契约测试明确排除 `docs/changes/archive/**`。
