# 规格：统一状态展示视图入口

## 验收矩阵

| 场景 | 测试 | 证据 | 风险 |
| --- | --- | --- | --- |
| status 展示统一到共享 view model | `status-view-unification-contract` | `status-view-unification-log` | 不覆盖全部长流程 shell 合同，依赖既有 Go 回归覆盖主要输出 |

### 需求：状态展示视图统一

系统必须让 human status、watch、runner JSON 和执行进度 checklist 复用同一套状态视图计算，避免 `app.go` 保留另一套展示状态来源。

#### 场景：status 展示统一到共享 view model

- **测试文件**：`docs/changes/archive/2026-06-14-27-统一状态展示视图入口/tests/test_status_view_unification_contract.sh`
- **真实数据来源**：现有 `internal/app/status_view_test.go` 构造的真实 `State`、DAG 节点、session、parallel artifact 和 stale lock 状态。
- **入口路径**：从仓库根目录运行 shell 合同测试，测试内部执行 `go test ./internal/app` 的 status/view/watch/runner JSON 回归。
- **关键断言**：
  - `app.go` 不再直接定义 checklist、visible session、planner session、session role 和 duration 汇总 helper。
  - `status_view.go` 和 `status_render.go` 共同承载展示 view 与 renderer。
  - status/view/watch/runner JSON 相关 Go 回归通过。
- **剩余风险**：合同测试不比较每一行中文文本的完整快照，执行阶段应保留或补充既有行为断言。
