# 优化 QA 交接与归档测试路径修复

## 场景

- 独立 QA 在 `qa_1` 报告问题后，状态机进入 `repair_2`。
- 提案归档后，从仓库根目录执行其验收脚本。

## 证据

- `TestRepairPromptCarriesLatestFailedQA` 初始失败：`repair_2` 提示词不包含 `qa-1.json`。
- 归档提案测试初始失败：脚本把 `docs/` 误判为仓库根目录，找不到 `internal/app/stage_role.go`。

## 根因与置信度

- 高置信度：prompt context 只收集前序 repair，没有按已完成 QA 阶段收集前序 QA；隔离的 repairer 会话无法获得 QA 具体 findings。
- 高置信度：验收脚本使用固定层级 `../../../..`，提案从活跃目录移入 `archive/<日期-名称>/` 后目录深度变化。

## 修复方案

- 仅为状态为 `completed` 的前序 QA 建立 artifact 上下文，并把最新 `qa-N.json` 注入下一轮 repair 提示词。
- 使用 `git rev-parse --show-toplevel` 从脚本所在目录解析仓库根目录，不依赖提案是否归档。

## 回归测试

- `go test ./internal/app -run TestRepairPromptCarriesLatestFailedQA -count=1`：通过。
- `bash docs/changes/archive/2026-07-26-44-简化审核修正为优化循环/tests/test_self_review_repair_loop.sh`：通过。
- `bash tests/specs/codex-workflow-cli/test_self_review_repair_loop_contract.sh`：通过。
- `go test ./...`：9 个包、190 项测试通过。

## 剩余风险

真实远端智能体是否逐项正确修复 QA findings 仍取决于模型能力；工作流现在保证该输入不会在会话隔离处丢失。
