# 45 收敛全量自查与 QA 定向修复闭环

## 问题

当前 `oz flow` 把 QA 前的全量优化与 QA 打回后的修正都建模为同一种 repair 循环，并由 `max_repair_iterations` 预先生成有限轮次。复杂提案可能在每轮继续扩大审查范围，即使验收命令持续通过，也会因轮次耗尽进入 `blocked_review_limit`；环境缺失还会被混入代码修复循环。

## 交付目标

- execution 后先进行一次可持续的全量自查；发现问题时修复、自测并再次全量确认，clean 后才移交独立 QA。
- QA 打回后只聚焦最新 QA findings 和失败验收项，修复者必须复跑失败测试及完整 required tests，通过后再移交下一轮 QA。
- 新运行不设修复轮次上限；循环由质量结果驱动，而不是由固定数字终止。
- 外部环境缺失与无进展分别进入可恢复阻塞状态，不伪装成代码失败，也不消耗虚构轮次。

## 非目标

- 不降低独立 QA 的最终放行权。
- 不允许通过删除、跳过或弱化 acceptance 合同来收敛。
- 不原地改写已 sealed 的旧运行；旧运行继续使用快照中的有限轮次状态机。
- 不承诺解决智能体模型本身的判断质量。

## 验收入口

执行阶段先运行：

```bash
bash docs/changes/archive/2026-07-27-45-收敛全量自查与QA定向修复闭环/tests/test_quality_delivery_loop.sh
```

该入口调用真实 `internal/app` 引擎集成测试，验证阶段顺序、prompt 上下文、自测门禁、12 轮以上收敛和环境阻塞恢复。

## 执行阶段默认上下文

默认读取本文件、`acceptance.json` 和 `docs/changes/archive/2026-07-27-45-收敛全量自查与QA定向修复闭环/tests/test_quality_delivery_loop.sh`。需要解决状态迁移、动态阶段或兼容问题时，再读取 `design.md`、`spec.md`、`task.md` 以及 `internal/app` 的 config、stage decision、prompt context、engine resume 和 graph 实现。
