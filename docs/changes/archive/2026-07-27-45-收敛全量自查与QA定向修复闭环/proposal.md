# 提案：收敛全量自查与 QA 定向修复闭环

## 为什么要改

`oz flow` 的最终目标是高效交付尽量少缺陷的实现。固定修复轮次把“资源保护”错误地变成“质量终止条件”：复杂变更可能仍在产生有效修复，却因为达到数字上限被判失败；同时 QA 打回后再次全量扩审，会让修复目标漂移并重复消耗时间。

最近的真实运行展示了这一问题：execution、validation 与 acceptance 持续通过，repair 仍连续产生新 findings，最终在第九轮被 `blocked_review_limit` 中断，独立 QA 没有获得最终放行机会。浏览器证据重采所需账户配置缺失又被混入 repair findings，使环境问题反复消耗优化阶段。

## 做什么

将新运行的质量流水线调整为：

```text
execution
    ↓
pre-QA 全量自查 ──发现并修复──↺ 全量自查
    ↓ clean 且自测通过
independent QA
    ├─ clean ───────────────→ archive
    └─ needs_fix
          ↓
       定向修复 QA findings
          ↓ 失败测试 + required tests 全通过
       independent QA ──────↺
```

具体变化：

- 区分 `pre_qa_audit` 与 `qa_targeted_repair` 两种修复意图及 prompt 上下文。
- 新运行使用动态循环，不再由 `max_repair_iterations` 预生成有限 DAG。
- QA 后 repair 只处理最新 QA artifact 中的 blocking findings、失败 acceptance IDs 和直接相关回归。
- repair artifact 必须记录实际修改、失败测试复跑和完整 required tests 结果；未通过时不能进入 QA。
- 环境前置缺失进入 `blocked_environment`，补齐后从原阶段恢复。
- 同一失败指纹且源码、测试和证据均无变化时进入 `blocked_stalled`，等待新信息；它不是轮次上限。

## 兼容策略

- 已 sealed 的旧 run 保持原 `max_repair_iterations` 快照和有限阶段图，确保可解释恢复。
- 新 run 不以 `max_repair_iterations` 作为终止条件；读取旧配置时给出迁移诊断，但不能静默改变 sealed run。
- CLI status、graph、contract JSON 和恢复命令同步表达全量自查、定向修复、环境阻塞及停滞阻塞。

## 影响范围

- `internal/app` 的配置解析、阶段决策、DAG/循环调度、prompt context、artifact gate、状态展示和恢复。
- `oz flow config/run/resume/restart/status/graph/contract`。
- 状态模型和运行证据元数据。

## 风险

- 无硬轮次上限可能导致无效循环，因此必须使用“是否有可证明进展”作为阻塞条件。
- 动态阶段会影响现有按最大轮次生成拓扑的代码，需要保持旧 sealed run 的读取兼容。
- 定向修复过窄可能遗漏连带回归，因此每轮仍必须执行完整 required tests，并由独立 QA 重新验收。
