# 任务

## 1. 先跑创建阶段合同

- [x] 1.1 运行 `bash docs/changes/9-精简-wo-status-watch-提案列表并修正watch清屏/tests/test_status_watch_proposal_list_contract.sh`，确认当前失败点是 status/watch 仍输出 batch/workflow header，且 watch spinner 仍在 header 或 running 行未替换。
- [x] 1.2 运行 `bash docs/changes/9-精简-wo-status-watch-提案列表并修正watch清屏/tests/test_watch_tty_clear_long_change_contract.sh`，确认当前失败点是窄终端长提案名刷新后仍有旧首行残留。

## 2. 调整 human 渲染结构

- [x] 2.1 从 `compactStatusLines` 或其调用侧移除 workflow header，让 workflow 行只包含阶段和子代理。
- [x] 2.2 新增或调整 proposal-list 渲染 helper，让单 run 输出也从 `- <change-name>` 开始。
- [x] 2.3 调整 batch status/watch 渲染，让 batch 只是多个提案列表组合，不输出 `b1 1/N` 顶部行。
- [x] 2.4 移除默认 status 顶部“正在查看最近一次批量工作流”提示，避免它挡在提案列表前。

## 3. 调整 watch marker

- [x] 3.1 让 `wo status` 继续使用静态 `→` 表示 running 阶段。
- [x] 3.2 让 `wo watch` 仅在 running 阶段行把 `→` 替换为当前 spinner 帧。
- [x] 3.3 更新旧测试中 “spinner header / 正文保留箭头” 的断言。

## 4. 修正 TTY 刷新

- [x] 4.1 改造 TTY watch 刷新，使用整屏刷新或按真实屏幕行数清理。
- [x] 4.2 保持非 TTY watch 继续追加多帧输出，方便脚本捕获 spinner。
- [x] 4.3 覆盖长中文提案名在窄终端中的多帧刷新，不再残留旧 header。

## 5. 验证

- [x] 5.1 重新运行两个创建阶段合同测试。
- [x] 5.2 运行 `go test ./internal/app`。
- [x] 5.3 运行现有 status/watch 相关 shell 回归测试，按新意图更新过期断言。
- [x] 5.4 运行 `oz validate 9-精简-wo-status-watch-提案列表并修正watch清屏 --json`。

## 执行记录

- 历史测试更新原因：旧 status/watch 回归仍断言 `→ b1`、`| b1`、`| w1` 顶部 header；本次新意图要求 human 输出直接从 `- <change-name>` 提案列表开始，spinner 只出现在 running 阶段行，因此只更新这些过期断言，保留失败摘要、别名解析、batch 优先和耗时列等原业务覆盖。
- `go test ./internal/app` 已运行；当前仅 `TestPlanningPromptReadsDiscussTemplate` 失败，失败内容为 prompt 模板期望不匹配，和本次 status/watch 渲染无关。
