# 设计：质量驱动的持续交付循环

## 状态机

新运行不再用有限的 `repair_1..repair_N` 配置决定可运行长度。阶段序号只用于持久化审计，不代表预算。

```text
execution
  → audit_1
      ├─ needs_more → audit_2 ...
      └─ clean + self-tests passed → qa_1
  → qa_N
      ├─ clean → archive
      └─ needs_fix → targeted_repair_N
                         ├─ self-tests failed → 留在本阶段
                         ├─ environment missing → blocked_environment
                         └─ self-tests passed → qa_(N+1)
```

全量自查与 QA 定向修复复用同一 repairer 的 backend-scoped 会话，但 prompt 模式、允许输入和退出合同不同：

| 模式 | 输入 | 可扩展范围 | 移交条件 |
| --- | --- | --- | --- |
| `pre_qa_audit` | acceptance、完整 diff、当前状态、验证结果 | 当前提案完整范围 | 本轮无新问题且所有 required tests 通过 |
| `qa_targeted_repair` | 最新 QA findings、失败 acceptance IDs、相关 diff | findings 及直接相关回归 | findings 已处理、失败测试与全部 required tests 通过 |

## 自测门禁

targeted repair 写入 artifact 后，执行器必须运行：

1. QA artifact 标记失败的测试或验收项；
2. `acceptance.json.required_tests` 全集；
3. 配置中额外声明的 validation commands。

状态只接受本轮源码/测试 diff 哈希对应的结果。任何命令失败、证据哈希过期或 required test 未运行，都留在 repair，不创建下一 QA stage。

## 不限轮次与停滞检测

新状态机没有最大修复次数。系统只在以下非质量原因下暂停：

- `blocked_environment`：缺少账号配置、密钥引用、外部服务、浏览器或其它明确前置条件。状态记录缺失项、发现命令和恢复入口。
- `blocked_stalled`：相邻两次相同失败指纹下，源码 diff、测试 diff、验证结果和 evidence 哈希均无变化。恢复必须提供新的代码、证据、配置或人工指令。

这两个状态不等价于 QA 失败或轮次耗尽；`oz flow resume/restart` 在条件变化后从原阶段继续。

## 动态阶段与持久化

- 新 run 的 workflow generation 使用新的质量循环版本，并动态追加 `audit_N`、`qa_N`、`targeted_repair_N`。
- 每个动态阶段由严格 repair/QA artifact、`state.json.quality_loop` 和对应 validation/acceptance-run artifact 共同记录 `mode`、`source_qa_artifact`、`finding_fingerprint`、`diff_hash`、实际测试结果和环境诊断；repair JSON 保持统一严格字段，哈希与测试绑定由执行器生成，不能由 agent 自报。
- graph 展示循环模板和当前实例，不要求预生成无限节点。
- latest completed QA 不再扫描到配置上限，而从持久 stage records 计算。

## 兼容

旧 sealed run 继续使用 `repair-v1` generation 与其 `max_repair_iterations`。新配置创建新 run 时迁移到质量循环；旧键保留可读诊断，不作为新运行终止条件。不得为了迁移而改写旧 state、artifact 或 prompt snapshot。

## 安全与证据

- repairer 不得修改 acceptance 来规避 QA findings。
- 环境诊断只记录缺失的配置路径或变量名，不记录密钥值。
- evidence 必须绑定当前 diff 哈希；需要真实环境的证据由明确 producer command 生成，缺环境时进入可恢复阻塞。
