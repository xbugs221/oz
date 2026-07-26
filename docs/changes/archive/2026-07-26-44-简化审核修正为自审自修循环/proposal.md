# 提案：简化审核修正为优化循环

## 为什么

当前 `review_N → fix_N → review_N+1` 由 reviewer、fixer 两个持续会话承担。角色内虽然复用会话，但角色之间仍依赖 artifact 转述，增加一次智能体调用、重复读取和语义漂移风险。

## 做什么

将主循环改为 `repair_N → repair_N+1`：每轮由同一个 repairer 会话完成自审、修正与验证，并写入结构化检查点。clean 后交给独立 QA；QA 不通过则继续下一轮 repair。配置大于 0 时，归档要求 repair clean 和 QA clean；配置为 0 时禁用 repair，没有 repair 检查点，仅由独立 QA clean 放行。

## 成功标准

- 同一运行中所有 repair 轮次复用同一个后端作用域 session。
- 状态机不再在 reviewer/fixer 两种会话间交接。
- 每轮结果可验证、可恢复、可供状态页读取。
- 轮次配置最大为 10，耗尽后安全阻塞。
- 新旧配置与历史运行的行为有自动化回归保护。
