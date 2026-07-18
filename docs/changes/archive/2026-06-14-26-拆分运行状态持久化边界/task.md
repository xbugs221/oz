# 任务

- [x] 1.1 先运行 `bash docs/changes/archive/2026-06-14-26-拆分运行状态持久化边界/tests/test_state_runtime_boundary_contract.sh`，确认失败点是目标边界尚未拆出。
- [x] 1.2 新增 `state_store.go`，迁出 state JSON 读写、run id 校验和合并写入逻辑。
- [x] 1.3 新增 `run_lock.go`，迁出 run lock、run 中止和 superseded archive 逻辑。
- [x] 1.4 新增 `prompt_context.go`，迁出 prompt snapshot、prompt template 渲染和 prompt context 构造逻辑。
- [x] 1.5 新增 `git_guard.go`，迁出 git snapshot、人工干预路径分类和 porcelain 解析逻辑。
- [x] 1.6 运行 `go test ./internal/app -count=1` 和合同测试，确认状态机行为保持不变。
- [x] 1.7 人工检查 `State` JSON 字段、runner JSON 输出和 run 目录结构没有变化。
