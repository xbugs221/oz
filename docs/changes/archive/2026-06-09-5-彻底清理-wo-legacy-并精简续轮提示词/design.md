# 设计

## 当前结构

```text
wo workflow runtime
  |
  +-- app.go
  |     +-- hidden "node" command still dispatches run-node helpers
  |     +-- run --engine only publicly accepts go-dag, but old node surface remains
  |
  +-- dagu.go
  |     +-- StartDaguJSON
  |     +-- writeRunDaguWorkflow / runDaguProcess
  |     +-- nodeRunStage / nodeGate / nodeFanin helpers
  |
  +-- graph.go
  |     +-- WorkflowSpec for go-dag
  |     +-- Dagu YAML structs/export functions still present
  |
  +-- config.go / state.go / stage_role.go
        +-- stages.writing fallback
        +-- prompts.writing fallback
        +-- prompt-snapshot.yaml normalizes writing
        +-- run/prompts/*.md fallback
        +-- Legacy role marker for old acceptance role

prompts-template
  |
  +-- wo-review.md
  |     +-- first turn has examples and full rules
  |     +-- resumed turn still repeats method and long static rules
  |
  +-- wo-fix.md
        +-- first turn has root-cause method
        +-- resumed turn is shorter but still repeats some startup constraints
```

问题不是默认路径会继续执行 Dagu，而是旧代码仍在运行时边界、测试边界和规格边界中存在。只要这些残留还在，后续维护者就必须判断它们是否仍是合同。

## 方案

### 删除 Dagu 运行时边界

删除或迁移 `dagu.go` 中仍被 `go-dag` 复用的业务 helper。保留下来的 helper 必须放在语义中性的文件中，例如：

```text
internal/app/node.go        # nodeResult、run-stage/gate/fanin 共享逻辑
internal/app/subagent.go    # subagent member 执行和 artifact gate
internal/app/go_dag.go      # 内嵌调度器
```

完成后，运行时代码中不得继续出现：

- `StartDaguJSON`
- Dagu CLI lookup 或 `dagu start`
- Dagu YAML workflow/exporter
- `dagu` run 子目录或 Dagu log
- 对用户或状态输出可见的 Dagu 诊断

隐藏 `wo node` 子命令随 Dagu executor 一起删除。`go-dag` 内部可以继续通过 Go 函数调用 stage/gate/fanin helper，不再需要暴露 run-node CLI。

### 收紧当前规格和长期测试

`docs/specs/codex-workflow-cli/spec.md` 应当描述唯一 `go-dag` 工作流合同，不再把 Dagu 写成一个被拒绝但仍需要解释的 engine。长期测试中用 fake Dagu CLI 证明“不调用 Dagu”的旧策略要改为直接验证当前行为：

```text
当前合同：wo 只有 go-dag
  -> 不需要在 PATH 中放 dagu
  -> 不需要提到 Dagu CLI
  -> 不需要维护 Dagu 专属回归脚本
```

归档目录保留历史审计材料，不参与本次扫描和清理。

### 移除 prompt legacy 兼容

配置读取只接受当前 stage/prompt key：

```text
planning
execution
review
qa
fix
archive
```

`writing` 不再作为 execution/fix 的 fallback。具体变化：

- `roleStageKinds()` 不再追加 `writing`
- `defaultStageOptionsByKind()` 不再写入 `writing`
- `normalizePromptConfig()` 删除或改为 no-op
- `mergeWorkflowConfigFile()` 不再手动映射 `prompts.writing`
- `workflowConfigFromInput()` 不再手动映射 `stages.writing`
- `runPromptTemplate()` 不再读取 `runs/<run-id>/prompts/<name>.md`
- `prompt-snapshot.yaml` 缺少目标角色 prompt 时直接报错
- 只用于历史兼容的 `Legacy` 角色字段和 acceptance role 从角色表移除

这会让旧 sealed run 不能继续靠备份 prompt 快照恢复。该破坏性变更符合本提案目标：清掉不再维护的备份兼容面。

### 精简续轮提示词

prompt 模板应按首轮和续轮分层：

```text
首轮:
  - 完整读取范围
  - 完整方法论
  - 完整 schema/示例
  - 完整质量门禁

续轮:
  - 当前角色会话 key
  - 本轮 stage / artifact 路径
  - 上一轮 review/QA/fix summary 路径
  - 当前 findings 和必要引用
  - 输出位置
  - 一句必要格式约束
```

`wo-review.md` 第 2 轮及之后不再重复“作为评审专家”、完整 clean/needs_fix checklist、workflow failure 大段说明和 JSON 示例。它只要求先核对上一轮 findings，再输出同 schema 的单个 JSON 对象。

`wo-fix.md` 第 2 轮及之后不再重复首轮根因分析方法论、验收合同保护长句和通用历史说明。它只要求修复当前 findings、记录验证结果、写入当前 `fix-N-summary.md`。

## 风险

- 仍在运行的旧 sealed run 如果只存在 legacy prompt 文件，将不能恢复。这是目标行为，但执行阶段需要让错误清楚指向缺少 `prompt-snapshot.yaml` 或缺少当前 prompt key。
- 删除 `wo node` 可能影响外部脚本。如果外部脚本仍调用该隐藏命令，应迁移为公开 `wo run/resume/status`。
- 长期测试删除 Dagu fake CLI 后，不能再通过“fake dagu 未被调用”证明默认路径；新的证明方式是运行时代码和当前长期测试中不再存在 Dagu 入口。
- 续轮提示词过度精简可能导致 artifact 格式波动。需要保留一句强约束：输出单个 JSON 或写入指定 summary 文件，并依靠现有 artifact gate 校验。
