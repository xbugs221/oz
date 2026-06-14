# 任务

- [x] 先运行 `bash docs/changes/33-拆分子智能体执行边界/tests/subagent_boundary_test.sh`，确认当前实现失败于结构边界缺失。
- [x] 创建 `subagent_attempt.go`，抽出单次执行尝试、retry 结果和超时处理。
- [x] 创建 `subagent_boundary.go`，移动 git/run artifact 只读边界校验。
- [x] 创建 `subagent_artifact.go`，移动 member artifact 读写、schema 校验和 captured text 兜底生成。
- [x] 创建 `subagent_prompt.go`，移动 prompt context、初次 prompt 和 retry prompt。
- [x] 保持 `nodeRunSubagent` 为薄编排入口，并运行 subagent/parallel 相关 Go 回归。
