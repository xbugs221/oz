# 规格：拆分 status view 渲染边界

## 验收矩阵

| 需求 | 场景 | required_tests | required_evidence | 关键断言 |
| --- | --- | --- | --- | --- |
| Status view 拆分 | 视图、耗时、渲染和 stale 判断被分离 | status-view-boundary | status-view-boundary-log | 目标文件存在，关键函数落在对应文件，status/watch Go 回归仍通过 |

### 需求：Status view 拆分

`internal/app` 必须把 status view 的模型构建、耗时计算、终端紧凑渲染和 stale 判断拆到独立文件，避免 `status_view.go` 继续承载所有职责。

#### 场景：视图、耗时、渲染和 stale 判断被分离

- 测试文件：`docs/changes/archive/2026-06-14-31-拆分status-view渲染边界/tests/status_view_boundary_test.sh`
- 真实数据来源：仓库当前 `internal/app` 生产代码和现有 `internal/app` Go 回归测试。
- 入口路径：执行 shell 契约测试，内部检查目标 Go 文件和运行 `go test ./internal/app` 的 status/watch 相关测试。
- 关键断言：`buildStatusView`、耗时计算、紧凑渲染、stale 判断必须分别落在对应文件；拆分后 `status_view.go` 不得继续是 1000 行级别职责集合；现有 status/watch 回归必须通过。
- 剩余风险：该测试不证明所有 status 输出文案完全不变，执行阶段仍需关注已有 shell 合同。
