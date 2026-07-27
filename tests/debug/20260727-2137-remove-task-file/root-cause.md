# 移除提案任务文件

## 场景

standard 提案在创建阶段被要求生成 `task.md`，执行器又依赖复选框推进 execution、status 与 archive。实现步骤因此被提前固化并反复写入 Git，与执行器已有 Todo 和持久运行态重复。

## 证据

- 提案 47 合同初次运行失败，`oz validate` 报 standard 缺少 `task.md`。
- `internal/ozcli` 的 validate、status、archive 直接读取任务文件或复选框。
- `internal/app/stage_artifact_gate.go` 把 execution 完成绑定到 `tasks.total/tasks.done`。
- 首轮移除文件门禁后，真实工作流测试发现 execution 会在调用执行器前被误判为已完成，证明“无文件产物”仍需明确的运行态完成信号。
- 独立复核发现 `agent_completed` 原先晚于 Git 快照写入；快照失败会丢失执行器已成功返回的事实，恢复时重复调用执行器。
- 独立复核还发现已封存旧运行会被新 active `task.md` 禁令永久打回，且 review JSON 残留无业务意义的 `tasks_verified`。
- 第二轮全量规格验证发现六个长期 shell 和主规格仍保存提案 42 之前的默认 parallel/subagent/fan-in 承诺；另有后端扫描自命中、`state.go` 固定行数阈值及 README 职责文案三类过期门禁。

## 根因与置信度

根因是创建合同、CLI 门禁和工作流运行态共同把静态任务清单误当成目标合同与执行进度事实源。真正的交付合同已经由 `brief.md`、`acceptance.json` 和测试表达，执行完成则应由执行器成功返回、运行态以及后续 validation/acceptance gate 证明。

置信度：高。旧依赖均可由源码搜索定位，且合同测试与真实工作流测试分别复现了结构校验失败和 execution 提前跳过风险。

## 修复方案

- standard 仅要求 `brief.md`、`proposal.md`、`design.md`、`spec.md`、`acceptance.json` 和 `tests/`。
- active 提案出现 `task.md` 时明确拒绝，历史 archive 中旧文件不迁移、不删除。
- execution 成功返回后在 `state.json` 记录 `agent_completed`，随后独立运行 validation 与 acceptance gate。
- 新运行写入 `proposal_contract_version=no-task-file-v1`；仅对版本字段缺失的旧 sealed 运行过滤这一条 task 禁令，其他 validation 错误仍照常失败。
- status、archive 和 execution artifact gate 删除任务复选框字段与判断。
- review schema 删除 `tasks_verified`，完成判断只依赖仍有意义的提案、测试、范围和运行时检查。
- 创建、执行、归档技能禁止创建或修改 `task.md`，动态计划只使用 Todo 或运行态。
- 复核旧子代理规格后按提案 42 分流：固定外置子代理、fan-in 和 parallel artifact 已明确废除，因此同步删除陈旧主规格与不可运行 shell；现行拒绝边界继续由 `test_remove_fixed_subagents_contract.sh` 覆盖。
- 将仍有当前价值的 graph、人工干预和 profile 模板规格迁到 execution/audit/targeted repair/QA 主阶段合同；清理主规格中默认 parallel、subagent row 和 MADA 并行角色等冲突段。
- 后端 allowlist 排除只用于断言禁词的规格脚本并直接校验 registry；Engine 边界改用职责符号归属而非会被后续需求自然突破的 `state.go` 行数；README 补齐 skill/change/acceptance/flow 职责。
- 更新三入口与 Release 长期规格，使其校验 standard 无任务文件产物和真实“尚未发布”说明，不再依赖旧 `task.md` 或空发布区假设。
- 归档提案 42 的 shell 是 review/fix 年代的历史快照，保持原样；后续 repair 与 audit/targeted repair 演进后的现行合同只运行根目录长期版 `tests/specs/codex-workflow-cli/test_remove_fixed_subagents_contract.sh`。

## 回归测试

- 提案合同覆盖 standard 无任务文件、active 任务文件拒绝、技能禁止规则和 execution artifact gate。
- `internal/ozcli` 覆盖 status 不暴露 tasks/task artifact、archive 保留历史任务文件。
- `internal/app` 覆盖 execution 在执行器返回前不得跳过、返回后由 `agent_completed` 放行，以及 status 使用 run state artifact。
- `internal/app` 覆盖 runner 成功后 Git 快照失败仍先持久化完成标记，恢复检查不会重新执行 agent；同时覆盖旧 sealed task 兼容与当前运行严格拒绝。
- 长期 shell 规格覆盖阶段 artifact 同会话重试、batch 归档产物修复续跑和默认工作流无固定外置子代理。

## 验证结果

- `bash docs/changes/47-移除并禁止提案任务文件/tests/test_no_task_file_contract.sh`：通过。
- oz CLI acceptance、严格合同、status 摘要和旧 acceptance 兼容规格：通过。
- 默认工作流、阶段 artifact 重试、batch artifact 修复续跑和固定外置子代理拆除规格：通过。
- `go test ./...`：通过。
- 枚举 `tests/specs/**/*.sh` 共 48 个，隔离每个脚本标准输入后结果为 48 PASS / 0 FAIL。
- 从提交创建的干净 worktree 重跑上述相关规格测试与 `go test ./...`：通过。

## 剩余风险

`oz validate` 只能约束进入校验、执行或归档边界的 active 提案；绕过 oz 直接写入文件的外部程序仍需由 Git 审阅识别。
