# 阶段两字展示名不一致

## 场景与证据

质量循环在 status/watch、JSON 和 graph 中分别显示“执行阶段”“全量自查”“定向修复”“独立测试”“归档阶段”。更新回归断言后，定向测试稳定复现 15 项旧名称失败。

## 根因与置信度

高置信度：状态视图和 graph 分别硬编码展示名，动态 graph 实例还直接把内部 stage 当作可见标签，缺少统一的人类名称映射。

## 修复方案

集中映射“执行、自查、修复、测试、归档”，仅替换 status/watch、JSON `Name` 与 graph 可见名称；保留内部 stage、节点 ID 和流转。

## 回归测试与验证

- 定向 Go 回归：17 项通过。
- `bash tests/specs/codex-workflow-cli/test_go_dag_graph_status_contract.sh`：通过。
- `bash tests/specs/codex-workflow-cli/test_compact_chinese_graph_and_iteration_limit.sh`：通过。
- `go test ./internal/app -count=1`：350 项通过。
- `go test ./... -count=1`：9 个包共 382 项通过。
- 从提交创建临时分离工作树后复跑两项 shell 合同与 `go test ./... -count=1`：全部通过。

## 剩余风险

历史 sealed run 的内部状态仍会按原协议恢复；本次不修改历史归档材料。
