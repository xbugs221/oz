# 任务

## 1. 契约测试先行

- [x] 1.1 运行 `bash docs/changes/7-统一-wo-status-watch-极简状态输出/tests/test_status_watch_compact_output_contract.sh`，确认当前失败点是 human 输出合同未实现。
- [x] 1.2 运行 `bash docs/changes/7-统一-wo-status-watch-极简状态输出/tests/test_status_json_observability_artifacts_contract.sh`，确认当前失败点是 JSON observability 缺失。
- [x] 1.3 运行 `go test ./internal/app`，记录旧 status/watch/JSON 回归中需要按新意图更新的断言。

## 2. 统一状态视图模型

- [x] 2.1 新增内部状态视图 row 结构，统一描述主阶段和 subagent 行。
- [x] 2.2 从 `State`、`WorkflowConfig`、`StageTimings`、`DAGNodes`、`Sessions` 和 run-local artifacts 组装状态视图。
- [x] 2.3 实现阶段名称和 subagent 短名映射，保留 JSON `full_name`。
- [x] 2.4 实现 marker 和耗时格式化：`-`、`→`、`✓`、`x` 和两位小数分钟。

## 3. Human 输出改造

- [x] 3.1 `wo status -wN` 使用 `→ wN` header 和固定列 rows。
- [x] 3.2 `wo watch -wN` 使用 spinner header，正文与 `status` 保持一致。
- [x] 3.3 batch 输出改为 `indicator bN current/total` 外层包装，并内嵌同一套 workflow 视图。
- [x] 3.4 移除 human status/watch 中的 engine 行、并行 group 汇总行和总耗时行。

## 4. JSON observability

- [x] 4.1 `wo status --run-id --json` 保留旧顶层 runner 字段。
- [x] 4.2 新增 `observability.engine`、`observability.rows` 和 `observability.artifacts`。
- [x] 4.3 为 execution/review/fix/qa/archive 输出固定 stage artifact 路径，即使文件尚未生成。
- [x] 4.4 为 subagent 输出 member artifact 和 fan-in group artifact 绝对路径。

## 5. 回归验证

- [x] 5.1 两个 change contract 测试通过。
- [x] 5.2 `go test ./internal/app` 通过。
- [x] 5.3 更新受新输出合同影响的根目录 shell 回归测试，不能删除业务断言。
- [x] 5.4 运行 `oz validate 7-统一-wo-status-watch-极简状态输出 --json`。
