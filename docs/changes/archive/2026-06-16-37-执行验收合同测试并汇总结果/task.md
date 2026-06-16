# 文件目的

本文件把验收合同执行器拆成可交付任务。任务范围刻意做成一个完整改造单元，避免只实现命令面而不接入 workflow gate。

## 任务

- [x] 1. 先运行 `bash docs/changes/37-执行验收合同测试并汇总结果/tests/test_acceptance_run_contract_surface.sh`，确认当前失败于 `run-acceptance` 命令面缺失。
- [x] 2. 先运行 `bash docs/changes/37-执行验收合同测试并汇总结果/tests/test_acceptance_run_success_contract.sh`，确认当前失败于目标命令缺失而不是测试语法错误。
- [x] 3. 先运行 `bash docs/changes/37-执行验收合同测试并汇总结果/tests/test_acceptance_run_failure_contract.sh`，确认当前失败于目标命令缺失而不是临时项目构造错误。
- [x] 4. 先运行 `bash docs/changes/37-执行验收合同测试并汇总结果/tests/test_acceptance_run_stage_gate_contract.sh`，确认当前缺少独立 acceptance run 边界和 gate 回归。
- [x] 5. 阅读 `internal/acceptance/acceptance.go`，确认现有合同字段、弱断言校验和 evidence producer 追溯规则。
- [x] 6. 阅读 `internal/app/acceptance.go`、`qa.go`、`qa_validate_command.go`，确认 QA artifact 和 acceptance matrix 现有边界。
- [x] 7. 阅读 `internal/app/validation.go` 和 `engine_stage.go`，确认现有 stage validation gate 写入和重试方式。
- [x] 8. 阅读 `internal/app/runner_contract.go` 和 `command_dispatch.go`，确认 runner JSON command surface 的扩展位置。
- [x] 9. 新增 `internal/app/acceptance_run.go` 或等价边界文件，文件开头写清业务目的。
- [x] 10. 定义 `AcceptanceRunResult`、`AcceptanceRunSummary`、`AcceptanceRunTestResult`、`AcceptanceRunEvidenceResult` 等 DTO。
- [x] 11. 为新增 DTO 字段写 Go doc，说明字段面向 runner 和 QA 复核。
- [x] 12. 实现 change 名称校验和 active change acceptance 路径定位，拒绝空 change、绝对路径和路径穿越。
- [x] 13. 复用现有 `ReadAcceptance` 或 `internal/acceptance.Read` 读取并校验 `acceptance.json`。
- [x] 14. 实现 result 目录规则 `test-results/acceptance-run/<change-name>/`，并保证目录可重复创建。
- [x] 15. 实现测试 id 到日志文件名的安全映射，避免 required test id 写出 result 目录。
- [x] 16. 实现单条 required test 执行函数，工作目录固定为 repo root。
- [x] 17. 单条测试执行时同时捕获 stdout 和 stderr 到独立日志。
- [x] 18. 单条测试结果记录 `id`、`source`、`path`、`command`、`status`、`exit_code`、`log_path` 和 `duration_ms`。
- [x] 19. 实现全部 required tests 顺序执行，失败不短路。
- [x] 20. 实现 evidence 检查函数，逐项检查 `required_evidence[].path` 是否存在且不是目录。
- [x] 21. evidence 结果记录 `id`、`kind`、`path`、`status`，缺失时标记为 `missing`。
- [x] 22. 汇总函数计算 `total`、`passed`、`failed`、`evidence_total`、`evidence_present`、`evidence_missing`。
- [x] 23. 汇总状态规则：全部测试通过且 evidence 全部存在时 `valid=true/status=passed`。
- [x] 24. 汇总状态规则：任一测试失败或 evidence 缺失时 `valid=false/status=failed`。
- [x] 25. 将完整 result JSON 写入 `test-results/acceptance-run/<change-name>/result.json`。
- [x] 26. JSON stdout 和 result 文件使用同一结构，避免 runner 与本地证据分叉。
- [x] 27. 新增 `runValidateAcceptanceCommand` 或等价命令函数，解析 `--change` 和 `--json`。
- [x] 28. 命令缺少 `--change` 或 `--json` 时返回明确中文用法错误。
- [x] 29. 将 `oz flow run-acceptance --change <change-name> --json` 接入 `app.Run` 或 repository command dispatch。
- [x] 30. 更新 `printHelp`，在人类命令或 Runner JSON 命令中展示 `run-acceptance`。
- [x] 31. 更新 `runnerCapabilities`，加入 `run-acceptance`。
- [x] 32. 为命令成功路径新增 `internal/app` Go 单测，使用临时 git repo 和真实 active change。
- [x] 33. 为命令失败路径新增 `internal/app` Go 单测，证明失败不短路且 JSON 保留所有结果。
- [x] 34. 为 evidence 缺失路径新增 Go 单测，证明测试通过但 evidence missing 仍返回 failed。
- [x] 35. 为非法 change 名称新增 Go 单测，证明不能路径穿越读取仓库外文件。
- [x] 36. 为日志路径安全映射新增 Go 单测，证明特殊 test id 不会逃逸 result 目录。
- [x] 37. 新增 `AcceptanceRunState` 或复用 `StageValidationState` 扩展字段，记录最后 result 路径和失败摘要。
- [x] 38. 在 execution 阶段完成后、进入 review 前调用 acceptance run gate。
- [x] 39. 在每轮 fix 完成后、进入下一轮 review 前调用 acceptance run gate。
- [x] 40. acceptance run gate 通过时不改变现有 stage decision 的 clean 路径。
- [x] 41. acceptance run gate 失败且仍可重试时，让同一阶段重新进入修复上下文。
- [x] 42. acceptance run gate 达到重试上限时进入明确阻断状态，并在 state error 中包含 result 路径。
- [x] 43. 将 acceptance run 失败结果注入下一轮 execution/fix prompt 的 validation failure 上下文。
- [x] 44. 确认 acceptance run gate 不影响 planning、review、qa、archive 的 artifact schema gate。
- [x] 45. 确认 `validation.go` 仍只负责用户配置的 validation commands，不直接实现 required_tests 主循环。
- [x] 46. 确认 `qa.go` 仍只负责 QA artifact 和 acceptance matrix 校验，不执行 required_tests 命令。
- [x] 47. 更新 `docs/specs/codex-workflow-cli/spec.md`，记录 `run-acceptance` runner 命令和 sealed run gate 行为。
- [x] 48. 更新 `README.md` 中验收合同或 workflow 说明，告诉维护者如何本地复现 required tests。
- [x] 49. 将本提案中稳定的 shell 合同按业务逻辑并入 `tests/specs/codex-workflow-cli/`。
- [x] 50. 运行本提案 4 个契约脚本、`go test ./internal/app ./internal/ozcli ./cmd/oz ./tests -count=1` 和必要的长期规格脚本。

## 验证条件

- [x] `oz flow run-acceptance --change <change-name> --json` 可执行。
- [x] 成功路径 JSON 和 result 文件一致。
- [x] 失败路径执行全部 required tests 并返回非零。
- [x] evidence 缺失能阻断 acceptance run。
- [x] `oz flow contract --json` 暴露 `run-acceptance`。
- [x] execution/fix 后 acceptance run gate 接入 sealed run。
- [x] 4 个创建阶段契约测试全部通过。
- [x] 相关 Go 回归测试通过。

