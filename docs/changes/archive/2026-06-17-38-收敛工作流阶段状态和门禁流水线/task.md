# 任务：收敛工作流阶段状态和门禁流水线

## 契约测试先行

- [x] 运行 `bash docs/changes/38-收敛工作流阶段状态和门禁流水线/tests/stage_gate_pipeline_contract_test.sh`，确认当前失败点来自目标边界缺失。
- [x] 记录失败日志到 `test-results/38-stage-gate-pipeline/contract.log`。
- [x] 运行 `go test ./internal/app`，确认基线测试状态。
- [x] 阅读 `internal/app/engine_run.go` 的 `runLoop` 主阶段完成流程。
- [x] 阅读 `internal/app/node.go` 的 `nodeRunStage` 主阶段完成流程。
- [x] 阅读 `internal/app/stage_decision.go`、`state_model.go`、`validation.go` 的状态/阶段常量。

## 阶段和状态语义

- [x] 新增阶段语义文件，说明文件功能目的。
- [x] 定义内部 workflow stage 表达，保留原始字符串。
- [x] 支持 `execution` 阶段解析。
- [x] 支持 `review_N` 阶段解析。
- [x] 支持 `qa_N` 阶段解析。
- [x] 支持 `fix_N` 阶段解析。
- [x] 支持 `archive` 阶段解析。
- [x] 对非法迭代阶段返回明确错误。
- [x] 增加 run status helper，识别 running、done、failed、blocked、interrupted、stale。
- [x] 增加 DAG node success/failure 规范化 helper。
- [x] 替换核心路径中重复的阶段前缀判断。
- [x] 保持 `stageIteration` 或兼容 wrapper，避免一次性改动过大。

## 主阶段门禁流水线

- [x] 新增主阶段门禁流水线文件，说明文件功能目的。
- [x] 定义流水线结果结构，表达 completed、retry、blocked、reason。
- [x] 将 artifact gate 检查纳入流水线。
- [x] 将 execution acceptance preflight 纳入流水线。
- [x] 将 required_tests acceptance run 纳入流水线。
- [x] 将 validation commands 纳入流水线。
- [x] 将 `markStageCompleted` 纳入流水线。
- [x] 将 `advance` 纳入流水线。
- [x] 保持 artifact gate failure 的同阶段 retry 行为。
- [x] 保持 acceptance run failure 的 retry/blocked 行为。
- [x] 保持 validation failure 的 retry/blocked 行为。
- [x] 保持 archive readiness 失败信息。

## 调用方收敛

- [x] 让 `runLoop` 调用主阶段门禁流水线。
- [x] 让 `nodeRunStage` 调用主阶段门禁流水线。
- [x] 移除 `nodeRunStage` 内重复的 acceptance/validation/advance 串联。
- [x] 保持 `nodeGate` 的现有职责不扩大。
- [x] 保持 DAG node result 的 completed/skipped 输出兼容。
- [x] 保持 progress 输出语义兼容。
- [x] 保持 runner JSON 的 stage/status 字符串不变。

## 测试和收尾

- [x] 为阶段解析新增表驱动 Go 测试。
- [x] 为 run status helper 新增表驱动 Go 测试。
- [x] 为流水线 acceptance failure 场景新增 Go 测试。
- [x] 为流水线 validation failure 场景新增 Go 测试。
- [x] 为流水线 archive clean 场景新增 Go 测试。
- [x] 更新或保留历史测试，确保新意图与旧断言一致。
- [x] 运行 `bash docs/changes/38-收敛工作流阶段状态和门禁流水线/tests/stage_gate_pipeline_contract_test.sh`。
- [x] 运行 `go test ./internal/app`。
- [x] 运行 `go test ./...`。
- [x] 确认没有改变 `docs/changes/archive/**` 历史提案。
