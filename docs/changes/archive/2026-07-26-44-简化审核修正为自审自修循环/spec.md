# 规格：自审自修循环

### 需求：审核与修正使用同一持续会话

#### 场景：问题在多轮自审自修中收敛

- **给定** execution 已完成，且最大修正轮次大于零
- **当** repairer 首轮仍发现并修正了问题
- **则** 下一轮继续复用同一个 backend-scoped repairer session
- **且** 状态机不得切换到独立 reviewer 或 fixer session
- **且** 每轮都写入可校验的 `repair-N.json`
- **测试**：`docs/changes/archive/2026-07-26-44-简化审核修正为自审自修循环/tests/test_self_review_repair_loop.sh`
- **真实数据来源**：`internal/app` 的真实角色映射、prompt context 和阶段决策函数
- **入口路径**：`oz flow run/resume`
- **关键断言**：repair 轮次共享 session；needs_more 进入下一 repair；clean 进入 QA
- **剩余风险**：具体 agent 后端的远端会话质量由现有 live 测试覆盖

### 需求：独立 QA 保留最终放行权

#### 场景：QA 失败返回同一自审自修循环

- **给定** repairer 已声明 clean
- **当** 独立 QA 仍发现 acceptance 未满足
- **则** 工作流进入下一轮 repair，并继续复用 repairer session
- **且** 最大修正轮次大于 0 时，只有 repair clean 与 QA clean 同时成立才能归档
- **且** 最大修正轮次为 0 时禁用 repair，不生成 repair artifact，仅由独立 QA clean 放行；QA needs_fix 时阻塞
- **测试**：`docs/changes/archive/2026-07-26-44-简化审核修正为自审自修循环/tests/test_self_review_repair_loop.sh`
- **真实数据来源**：`internal/app` 的真实阶段决策与归档 readiness gate
- **入口路径**：`oz flow run/resume`
- **关键断言**：QA needs_fix 不归档；QA clean 才进入 archive
- **剩余风险**：QA 检查质量仍受模型能力影响，但其会话与 repairer 隔离

### 需求：循环有明确上限与迁移边界

#### 场景：轮次耗尽或读取旧运行

- **给定** 新配置声明修正轮次，或 sealed run 使用旧 review/fix 状态机
- **当** 配置超过 10、最后一轮仍未 clean，或恢复旧 run
- **则** 超范围配置被拒绝，轮次耗尽后工作流阻塞且不得归档
- **且** 旧 sealed run 按快照继续旧状态机，不被静默改写
- **且** 旧配置迁移和冲突规则给出确定结果
- **测试**：`docs/changes/archive/2026-07-26-44-简化审核修正为自审自修循环/tests/test_self_review_repair_loop.sh`
- **真实数据来源**：真实 YAML 配置加载、sealed state 与阶段图生成逻辑
- **入口路径**：`oz flow config/run/resume/graph/status`
- **关键断言**：0～10 合法、11 非法；0 轮禁用 repair 并保留独立 QA；末轮失败阻塞；旧 run 可恢复
- **剩余风险**：ozw 新 repair 展示需与其消费仓库联动发布
