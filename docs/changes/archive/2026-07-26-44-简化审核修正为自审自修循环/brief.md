# 简化审核修正为优化循环

## 用户问题

`oz flow` 当前在独立 reviewer 与 fixer 会话之间往返，通过审核 JSON 和修正摘要搬运上下文。多轮运行会增加调用开销，并可能因报告压缩造成上下文漂移。

## 交付目标

- 用同一个 `repairer` 角色会话连续执行“检查当前实现、修正问题、验证结果”。
- 每轮保留可恢复、可审计的结构化 `repair-N.json` 检查点，但不再把审核报告交给另一个修正会话。
- 优化 clean 后进入独立 QA；QA 失败时回到下一轮同一 repairer 会话。
- 默认最多 5 轮，配置允许 0～10 轮；达到上限仍不 clean 时阻塞，不归档。
- 为旧 `review`/`fix` 配置和历史 sealed run 提供明确兼容边界。

## 非目标

- 不把 QA 合入优化会话。
- 不允许 repairer 自行放行归档。
- 不在单次不可恢复的智能体调用内隐藏全部十轮迭代。

## 验收入口

先运行 `bash docs/changes/archive/2026-07-26-44-简化审核修正为优化循环/tests/test_self_review_repair_loop.sh`，再运行 `go test ./...`。执行阶段默认读取本简报、`acceptance.json` 与该契约测试。
