# 33-拆分子智能体执行边界

当前 `internal/app/subagent.go` 同时负责子智能体参数解析、prompt 组装、runner 调用、三次重试、artifact 兜底生成、schema 校验、git/run artifact 只读边界和 session merge。该文件是并行 helper 的关键路径，后续修改 retry 或 artifact 合同时风险较高。

本次交付目标是把 subagent 执行编排拆成清晰边界，并保持并行 helper 行为和只读保护不变。非目标是不改 helper prompt 语义、不调整 artifact JSON schema、不放宽只读边界。

执行阶段默认先运行 `bash docs/changes/33-拆分子智能体执行边界/tests/subagent_boundary_test.sh`，确认当前实现失败于结构边界缺失，再完成拆分并让测试通过。验收证据写入 `test-results/33-subagent-boundary/contract.log`。
