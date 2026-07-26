# 任务：实现自审自修循环

## 1. 先固定合同

- [x] 运行本提案契约测试并确认它因尚无 repair 状态机而失败。
- [x] 补充 `internal/app` 单元测试，覆盖角色会话、阶段决策、轮次上限和归档 gate。

## 2. 合并角色与阶段

- [x] 引入 repair 角色、`repair_N` durable stage 与统一 prompt context。
- [x] 让每轮复用 backend-scoped repairer session，生成并校验 `repair-N.json`。
- [x] 删除新运行中的 reviewer/fixer 往返，同时保持旧 sealed run 可恢复。

## 3. 保留独立 QA

- [x] repair clean 后进入独立 QA，QA needs_fix 后进入下一轮 repair。
- [x] 收紧 archive readiness：正轮模式必须同时存在 repair clean 与 QA clean；零轮模式禁用 repair，仅由独立 QA clean 放行。
- [x] 达到最后一轮仍失败时置为阻塞，不归档。

## 4. 配置与展示合同

- [x] 增加 `max_repair_iterations`、`stages.repair`、`prompts.repair`，上限为 10。
- [x] 实现旧配置弃用迁移、冲突拒绝和 sealed run 兼容。
- [x] 更新 config、contract、graph、status/watch、prompt snapshot 与 ozw 消费规格。

## 5. 验证

- [x] 运行提案契约测试、`go test ./...` 和相关 shell specs。
- [x] 保存真实 run 的状态快照与 runtime log，证明 session 复用、QA 隔离和上限阻塞。
