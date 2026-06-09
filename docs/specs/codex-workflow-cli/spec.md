# codex-workflow-cli 规格

## 目的

定义 `wo` CLI 的工作流行为，包括 oz change 选择与批量执行、sealed run 的自动推进与断点恢复、agent tool 驱动、审核修正循环的提示词历史范围、配置管理与快照兼容，以及 runner contract 和状态查询接口。

## 需求

### 需求：oz change 选择

系统必须支持用户在终端中选择已有 `docs/changes/<change-name>` active change，且必须排除 `docs/changes/archive` 和隐藏目录。

#### 场景：列出可执行变更

- **当** 用户选择“选择已有变更提案”
- **则** 系统列出 `docs/changes` 下所有 active change 名称
- **则** 列表不包含 `archive` 和以 `.` 开头的目录

#### 场景：批量选择多个变更

- **当** 用户选择“选择已有变更提案”
- **且** 输入 `1,3` 或 `1-3` 这样的多选表达式
- **则** 系统创建 用户状态目录中的 `batches/<batch-id>/state.json`
- **且** 只启动一个 detached batch worker

#### 场景：选择单个变更创建单项队列

- **当** 用户选择“选择已有变更提案”
- **且** 只输入单个编号
- **则** 系统创建 用户状态目录中的 `batches/<batch-id>/state.json`
- **且** `changes` 只包含所选 change
- **且** 只启动一个 detached batch worker
- **且** 用户后续可以向该运行中队列追加其他 active change

#### 场景：非法批量输入

- **当** 用户输入空值、非数字、越界编号或非法范围
- **则** 系统返回明确错误
- **且** 不创建新的 `用户状态目录 runs/`
- **且** 不创建新的 `用户状态目录 batches/`

### 需求：批量串行执行

系统必须将一次批量选择拆成多个独立 sealed run，并按 change 名称数字前缀升序串行执行。

#### 场景：按编号排序

- **给定** active change 包含 `5-c`、`3-a`、`4-b`
- **当** 用户批量选择这三个 change
- **则** batch state 中的执行顺序为 `3-a`、`4-b`、`5-c`
- **且** 无数字前缀的 change 排在有数字前缀的 change 后

#### 场景：前一个完成后才启动下一个

- **当** batch worker 正在执行队列
- **则** 当前 change 的 run 状态达到 `done` 前不得创建后续 change 的 run
- **且** 每个队列内 run 的 `state.json` 必须记录可选的 `batch_id`、`batch_index` 和 `batch_total`

#### 场景：失败停止

- **当** 当前 run 状态为 `failed`、`aborted`、`blocked_review_limit` 或 `blocked_validation_limit`
- **则** batch 状态变为 `failed`
- **且** batch state 记录当前 change、run id 和错误信息
- **且** 后续 change 不得启动

#### 场景：恢复和中止未完成 batch

- **当** 用户状态目录中的 `batches/<batch-id>/state.json` 存在且状态未完成，例如 `running` 或 `failed`
- **则** 无参数启动 `wo` 时必须优先提示未完成 batch
- **且** 用户可以选择恢复 batch、中止 batch 或开始新的 run
- **当** 用户恢复 batch
- **则** 系统跳过已完成 run，继续当前未完成 run 或创建下一个 run
- **当** 用户中止 batch
- **则** batch 状态写为 `aborted`
- **且** 当前未完成 run 被中止或保持可明确识别的中止状态

#### 场景：追加变更到运行中 batch

- **当** 存在一个状态为 `running` 的 batch
- **且** 用户选择“追加变更提案”
- **则** 系统列出尚未在该 batch 队列中的 active changes
- **且** 用户可以选择一个或多个 change（支持 `1,3`、`1-3` 和独立输入 `a` 全选）
- **则** 系统把所选 change 追加到 batch 队尾
- **且** 追加的 change 按数字前缀升序排列
- **且** 已存在于 batch 的 change 不进入候选列表，不重复追加
- **且** 没有可追加 change 时不得进入编号选择
- **且** 系统输出追加列表
- **且** `current_index` 不得改变
- **且** 当前正在执行的 run 不得被中断
- **且** 不得启动第二个 batch worker

#### 场景：人类快捷命令创建单项队列

- **给定** active change 包含 `1-a`
- **当** 用户运行 `wo --run 1-a`
- **则** 系统创建 用户状态目录中的 `batches/<batch-id>/state.json`
- **且** `changes` 为 `["1-a"]`
- **且** 不得创建脱离队列的普通新 run 作为人类默认执行对象

#### 场景：非运行中 batch 拒绝追加

- **当** batch 状态为 `done`、`failed` 或 `aborted`
- **且** 用户尝试追加 change
- **则** 系统返回明确错误
- **且** 不得修改 batch state

### 需求：交互式规划入口

系统必须支持用户先进入配置指定的 agent tool 规划会话，并在会话退出后重新扫描 active change 供用户选择。

#### 场景：规划后选择新提案

- **当** 用户选择“进入规划模式”
- **则** 系统启动 planning 阶段 effective tool 对应的 CLI 会话
- **当** 用户退出 agent tool CLI
- **则** 系统扫描 `docs/changes` 并要求用户选择一个 change 后才能进入 sealed run

### 需求：Sealed run 自动推进

系统必须在用户确认开始执行后禁止人工介入，并按配置化 execution、review、qa、fix、archive 阶段自动推进。

#### 场景：提案验收合同先行

- **当** 用户启动 sealed run
- **则** 对应 `docs/changes/<change>/acceptance.json` 必须存在并通过严格 JSON 校验
- **且** `required_tests` 至少包含一项测试
- **且** `required_tests` 必须复用 oz-create 写入 `docs/changes/<change>/tests/` 的契约测试，必要时补充根目录端到端或回归测试
- **且** `required_evidence` 必须记录 QA 需要采集的截图、trace、network、console、runtime log 或等价证据
- **且** 系统不得接受 Markdown-only 的验收说明作为 sealed run 前置合同

#### 场景：审核提前通过

- **当** `review_i.json` 的 `decision` 为 `clean`
- **且** review artifact 通过严格校验
- **则** 系统跳过后续 review 和 fix
- **且** 进入同轮 `qa_i` 阶段
- **且** QA clean 后进入 archive 阶段

#### 场景：QA 要求修复

- **当** `qa_i.json` 的 `decision` 为 `needs_fix`
- **且** `i < max_review_iterations`
- **则** 系统进入 `fix_i`
- **则** 修复完成后进入 `review_{i+1}`

#### 场景：审核要求修复

- **当** `review_i.json` 的 `decision` 为 `needs_fix`
- **且** `i < max_review_iterations`
- **则** 系统进入 `fix_i`
- **则** 修复完成后进入 `review_{i+1}`

### 需求：agent tool JSONL 事件驱动

系统必须通过当前阶段 effective agent tool 的 stdout JSONL 事件流抽取 session id，但不在 run 目录重复保存 agent JSONL 日志。

#### 场景：新 executor 会话启动

- **当** 系统执行 execution 阶段
- **则** 系统调用当前阶段 effective agent tool 的非交互运行命令
- **则** 系统从 `thread.started.thread_id` 保存 executor session id
- **且** 不创建 用户状态目录中的 `runs/<run-id>/logs/`

#### 场景：resume fixer 会话

- **当** 系统执行 fix 阶段且已有 fixer session id
- **则** 系统按相同 agent tool 和 fixer role 查找 session id 并续跑
- **则** 系统等待进程退出并继续从 stdout JSONL 抽取 session id

#### 场景：fixer session 按 backend 保存

- **当** 系统执行任意 `fix_N` 阶段
- **且** 当前阶段 effective tool 为 `codex`、`opencode` 或 `pi`
- **则** 系统必须把 stdout JSONL 中抽取的可恢复 session id 保存到 `<tool>:fixer`
- **且** 不得保存为 `<tool>:executor`
- **且** runner 返回 session id 后必须再次兜底更新同一个 `<tool>:fixer` key

#### 场景：多轮 fix 复用 fixer session

- **给定** `fix_1` 已保存 `<tool>:fixer`
- **且** `review_2` 仍返回 `needs_fix`
- **当** 系统执行 `fix_2`
- **则** 系统必须把 `<tool>:fixer` 对应 session id 传给当前 backend 的 resume 参数
- **且** `fix_2` 继续把新抽取或返回的 session id 保存到 `<tool>:fixer`

#### 场景：Pi session header

- **当** 当前阶段的 effective tool 为 `pi`
- **且** Pi JSONL 输出 `{"type":"session","id":"pi-session-1"}`
- **则** 系统保存当前角色的 `pi:<role>` session id 为 `pi-session-1`
- **且** 状态展示使用统一 agent session progress 格式

### 需求：断点继续

系统必须在程序中断后通过 用户状态目录中的 `runs/<run-id>/state.json` 恢复未完成 run。

#### 场景：当前阶段产物已存在

- **当** 用户选择恢复未完成 run
- **且** 当前阶段要求的产物已经存在且格式合法
- **则** 系统跳过该阶段并推进到下一阶段

#### 场景：当前阶段产物缺失

- **当** 用户选择恢复未完成 run
- **且** 当前阶段要求的产物不存在或格式不合法
- **则** 系统使用该 agent tool 和角色保存的 session id 重跑当前阶段

### 需求：人工干预中止

系统必须在 sealed run 中检测无法归因的工作区变化，并中止工作流。

#### 场景：阶段边界发现未知变化

- **当** sealed run 已开始
- **且** 阶段开始前发现工作区变化无法归因于上一阶段
- **则** 系统把 run 状态写为 `aborted_manual_intervention`
- **则** 系统停止继续调用 agent tool

### 需求：仓库级工作流配置

系统必须支持从内置默认、`~/wo.yaml`、仓库 `wo.yaml` 或 `wo.yml` 读取工作流配置，并在缺失配置时使用内置默认值。

#### 场景：初始化默认配置

- **当** 用户在仓库根目录调用 `wo config`
- **且** 仓库根目录不存在 `wo.yaml` 或 `wo.yml`
- **则** 系统创建默认 `wo.yaml`
- **且** 默认配置包含 `max_review_iterations: 5`
- **且** 默认配置包含 `workflow.stages.planning/execution/review/qa/fix/archive`
- **且** 每类会话配置包含 `cli` 和 `reasoning`
- **且** 默认配置写入 `planning.reasoning: xhigh`、`execution.reasoning: low`、`review.reasoning: high`、`qa.reasoning: high`、`fix.reasoning: low`、`archive.reasoning: low`
- **且** 默认配置写入 `validation.max_attempts_per_stage: 3`
- **且** 默认配置写入空的 `validation.commands: []`
- **且** 默认配置写入 `prompts.planning/execution/review/qa/fix/archive`
- **且** 不创建 `.wo/`
- **且** 后续规划或 sealed run 能读取其中的 `wo.workflow` 配置

#### 场景：初始化全局默认配置

- **当** 用户调用 `wo config --global`
- **且** `~/wo.yaml` 不存在
- **则** 系统创建 `~/wo.yaml`
- **且** 不创建 `~/.wo/`

#### 场景：避免覆盖配置

- **当** 仓库根目录已经存在 `wo.yaml` 或 `wo.yml`
- **且** 用户调用 `wo config`
- **则** 系统报错
- **且** 不覆盖已有配置

#### 场景：旧 init 不兼容

- **当** 用户调用 `wo init`
- **则** 系统返回非零退出
- **且** 错误信息提示改用 `wo config`

#### 场景：旧 install 不兼容

- **当** 用户调用 `wo install`
- **则** 系统返回非零退出
- **且** 错误信息提示 prompt 已内嵌在 `wo.yaml` 中

#### 场景：读取 YAML 配置

- **当** 仓库根目录存在 `wo.yaml`
- **且** 用户进入规划模式或启动 sealed run
- **则** 系统读取 `wo.workflow` 配置
- **且** 校验 reasoning 只能是 `low`、`medium`、`high`、`xhigh`
- **且** 校验 `max_review_iterations` 是非负整数
- **且** 校验 `validation.commands` 中的命令必须是非空字符串

#### 场景：配置文件冲突

- **当** 仓库根目录同时存在 `wo.yaml` 和 `wo.yml`
- **则** 系统启动前报错
- **且** 不创建 sealed run

#### 场景：无配置文件

- **当** 仓库根目录不存在 `wo.yaml` 或 `wo.yml`
- **则** 系统使用内置默认配置

### 需求：OMO 风格并行增强配置

系统必须在保留 sealed run 主状态机的前提下，提供默认关闭的 `workflow.parallel` prompt-driven artifact contract。`workflow.subagents` 是同一结构的 OmO 命名别名，`members[].cli` 是 `members[].tool` 的别名。并行组只能作为 planning/execution 前置上下文、review 输入或 QA 输入，不得绕过 review、QA、fix 或 archive gate。`members[].tool` 和 `members[].subagent` 只作为提示词角色线索，不触发额外 CLI 发现、subagent 调度或 backend 专属参数。

#### 场景：默认配置包含并行能力骨架

- **当** 用户运行 `wo config`
- **则** 生成的 `wo.yaml` 包含 `wo.workflow.parallel`
- **且** `enabled` 默认为 `true`
- **且** 包含 `planning_context`、`implementation_context`、`review`、`qa` 四类 groups
- **且** 默认成员名称包含“需求分析员”“代码库侦察员”“外部资料研究员”“目标核对审核员”“CLI/API 测试员”等直观职责名
- **且** 不把 Sisyphus、Prometheus、Metis、Momus、Oracle 或 Explore 等内部 agent 名称作为主要用户可见成员名称

#### 场景：并行层关闭时行为兼容

- **给定** 默认生成的 `wo.yaml`
- **当** 用户没有显式启用 `workflow.parallel.enabled`
- **则** sealed run 仍按现有串行阶段推进
- **且** review、QA、fix 和 archive 的 artifact gate 规则不变

#### 场景：导出 workflow 图

- **给定** `workflow.parallel.enabled` 或 `workflow.subagents.enabled` 为 `true`
- **且** 配置了 `before_execution`、`before_review` 或 `before_qa` 并行组
- **当** 用户运行 `wo graph --change demo --format json`
- **则** 输出必须是合法 WorkflowSpec JSON
- **且** 包含 `main_stage`、`subagent`、`fanin` 和 `gate` 节点
- **当** 用户运行 `wo graph --change demo --format mermaid`
- **则** 输出必须显示 fan-out/fan-in、review clean/needs_fix、QA clean/needs_fix 和 archive gate
- **且** 可选 graph format 只有 `json` 和 `mermaid`
- **且** 图只描述 workflow spec，不暴露内部调度命令
- **且** 不得直接拼接 `codex exec`、`pi --mode json` 或 `opencode run`
- **且** 导出图时不得创建 run、batch 或 agent session

#### 场景：workflow engine 只支持 go-dag

- **给定** 当前仓库存在 active change 和合法 `acceptance.json`
- **当** 用户运行 `wo run --change demo --json`
- **则** 默认 engine 必须是内嵌 `go-dag`
- **且** `wo run --change demo --engine unknown --json` 必须被拒绝，并引导用户使用 `go-dag`
- **且** `workflow.engine` 只允许为空或 `go-dag`

#### 场景：go-dag subagent artifact schema retry

- **给定** go-dag 在进程内调度 `before_review` 的 subagent 节点，参数包含 run id、member name、stage 和 iteration
- **当** subagent 正常退出但写出的 member artifact 中 `evidence` 为对象数组而非字符串数组
- **则** 系统必须 resume 同一 subagent session 要求只重写 `SUBAGENT_OUTPUT`
- **且** 修正 prompt 必须包含字段名、期望类型和 artifact 路径
- **且** 最多重试 3 次，仍失败才将 run 标记为 `failed`
- **且** 修正通过后 fan-in 才能继续读取该成员产物
- **则** `wo` 必须渲染只读 subagent prompt，包含 `SUBAGENT_GROUP`、`SUBAGENT_NAME`、`SUBAGENT_PURPOSE` 和 `SUBAGENT_OUTPUT`
- **且** subagent 必须写出单成员 JSON artifact，不能修改源码或 worktree
- **且** 单成员 JSON 顶层只允许 `name`、`purpose`、`status`、`summary`、`evidence`、`findings`
- **且** `evidence` 必须是字符串数组
- **且** `findings[]` 每项只允许 `title`、`severity`、`evidence`、`recommendation` 四个字符串字段
- **且** prompt 必须明确禁止 `category`、`description`、`detail`、`location`、`level`、`type` 等额外字段
- **且** `findings[].severity` 最终只允许 `blocker`、`major`、`minor`
- **且** `critical/blocker` 归一为 `blocker`，`high/medium/major` 归一为 `major`，`low/nit/minor/info/informational/note/warning` 归一为 `minor`
- **当** go-dag 在进程内执行 fan-in 节点
- **则** `wo` 必须汇总为既有 `parallel-implementation-context.json`、`parallel-review-N.json` 或 `parallel-qa-N.json`
- **当** go-dag 在进程内执行 gate 节点
- **则** review/QA clean 不得忽略 gate_input subagent 的 blocker/major finding 或成员失败
- **且** clean review/QA 必须跳过未激活 fix 分支，needs_fix 必须激活下一轮 fix/review/QA 分支

#### 场景：并行层开启时主阶段不变

- **给定** 用户启用 `workflow.parallel.enabled: true`
- **当** sealed run 进入 execution、review 或 QA
- **则** 当前主 agent 可以按 prompt 读取 `workflow_config.parallel.groups` 并写入对应 `parallel-*.json`
- **且** 当前主阶段仍分别产出 execution 结果、`review_i.json` 或 `qa_i.json`
- **且** sealed run 不为并行成员创建额外 session 或调用额外 `AgentRunner`
- **且** archive gate 仍只接受现有证据链完整后完成

#### 场景：planning 和 execution 并行上下文进入后续 prompt

- **给定** `planning_context` 或 `implementation_context` 组已启用
- **当** sealed run 进入后续 execution、review 或 QA prompt
- **则** prompt 必须引用当前 run 目录中的 `parallel-planning-context.json` 或 `parallel-implementation-context.json`
- **且** prompt 必须说明 `tool/subagent` 只作为提示词角色线索，不作为 CLI 参数
- **且** advisory 成员失败时必须记录失败摘要并继续主阶段
- **且** required 成员失败时必须阻断阶段完成或推进

#### 场景：并行成员 tool 不参与 sealed run 工具发现

- **给定** 所有主阶段使用默认 `codex`
- **且** `workflow.parallel.enabled: true`
- **且** 并行成员配置 `tool: pi` 和 `subagent: explore`
- **当** sealed run 做工具发现
- **则** 系统只要求主阶段需要的 `codex`
- **且** 不得因为 prompt-only 并行成员要求本机存在 `pi`

#### 场景：Pi 参数不携带并行 subagent metadata

- **给定** 主阶段使用 `cli: pi`
- **且** 并行成员包含 `subagent: explore`
- **当** 系统构造 Pi sealed run 参数
- **则** 参数不得包含 `--subagent`、`--agent`、`explore` 或 OpenCode 专属 agent 参数

#### 场景：并行 review gate 不得被 clean 忽略

- **给定** `review` 并行组已启用
- **当** `parallel-review-i.json` 中任一配置成员缺失、成员失败，或 gate_input 成员报告 blocker/major finding
- **则** 同轮 `review_i.json.decision` 不得为 `clean`

#### 场景：并行 QA 覆盖 acceptance 合同

- **给定** `qa` 并行组已启用
- **当** 当前轮 QA 开始
- **则** 当前主 agent 按 `workflow_config.parallel.groups.qa` 的职责生成 `parallel-qa-i.json`
- **且** `qa_i.json.acceptance_matrix` 必须逐项覆盖 `acceptance.json.required_tests` 和 `required_evidence`
- **且** 任一配置成员缺失、成员失败、blocker/major finding 或 required evidence 缺失时，`qa_i.json.decision` 不得为 `clean`

### 需求：默认配置经过审计并保持一致

系统必须审计默认 `wo.yaml` 相关配置，并保证所有默认来源一致。

#### 场景：生成默认配置

- **当** 用户运行 `wo config`
- **则** 生成的 `wo.yaml` 必须使用审计后的默认工作流值
- **且** 生成的 prompt 必须使用精简后的默认模板
- **且** 生成的默认值必须与内置 `DefaultWorkflowConfig` 一致

#### 场景：默认值发生调整

- **给定** 实现阶段调整了 `max_review_iterations`、阶段 reasoning、fast 或 validation 默认值
- **当** 用户查看 README 或规格文档
- **则** 文档中的默认值必须与 `wo config` 生成结果一致
- **且** 测试必须覆盖该默认值变化带来的用户可见行为

#### 场景：默认值保持不变

- **给定** 实现阶段审计后决定保留某个默认值
- **当** 审阅者查看变更
- **则** 任务或设计说明必须记录保留原因
- **且** 测试仍必须保证内置默认和生成 YAML 不发生分叉

### 需求：阶段级 reasoning 和 fast mode

系统必须允许用户配置 planning、execution、review、qa、fix、archive 六类会话的 reasoning depth；未知阶段键必须在配置读取阶段被拒绝。

#### 场景：传递 reasoning depth

- **当** 当前阶段的 effective reasoning 为 `high`
- **则** 系统调用 Codex 时传递 `-c model_reasoning_effort=high`

#### 场景：开启 fast mode

- **当** 当前阶段的 effective fast 为 `true`
- **则** 系统调用 Codex 时传递 `--enable fast_mode`

#### 场景：关闭 fast mode

- **当** 当前阶段的 effective fast 为 `false`
- **则** 系统调用 Codex 时传递 `--disable fast_mode`

### 需求：阶段级 agent tool 和模型

系统必须允许用户配置 planning、execution、review、qa、fix、archive 六类会话的 agent CLI 和模型，且未配置时默认使用 Codex。未知阶段键必须在配置读取阶段被拒绝。内置 agent tool 包含 `codex`、`opencode` 和 `pi`，不支持把 `pi-ai` 作为 `pi` 的别名。

#### 场景：配置工具和模型

- **当** 当前阶段的 effective tool 为 `opencode`
- **且** effective model 非空
- **则** 系统调用 OpenCode 并传递 `-m <model>`

#### 场景：配置 Pi 工具和模型

- **当** 当前阶段的 effective tool 为 `pi`
- **且** effective model 非空
- **且** effective reasoning 为 `high`
- **则** 系统调用 Pi 并传递 `--model <model>` 和 `--thinking high`
- **且** sealed run 调用 Pi 时传递 `--mode json`

#### 场景：未知工具

- **当** 任一 effective stage 的 `cli` 没有后端适配
- **则** 系统在创建 sealed run 前报错
- **且** 不创建 `用户状态目录 runs/` 运行态文件

#### 场景：Pi 别名不兼容

- **当** 任一 effective stage 配置 `cli: pi-ai`
- **则** 系统返回无效或未知 agent tool 错误
- **且** 不把 `pi-ai` 自动映射为 `pi`

#### 场景：跨工具 session 隔离

- **当** execution 使用 `opencode` 保存了 executor session
- **且** fix 使用 `codex`
- **则** fix 不得复用 OpenCode executor session
- **且** fix 必须开启并保存 Codex fixer session

#### 场景：OpenCode 忽略 fast

- **当** 当前阶段的 effective tool 为 `opencode`
- **则** 系统不得向 OpenCode 传递 Codex fast mode 参数

#### 场景：Pi 忽略 fast

- **当** 当前阶段的 effective tool 为 `pi`
- **则** 系统不得向 Pi 传递 Codex fast mode 参数

### 需求：Pi 工具发现和失败边界

系统必须只检查当前路径实际需要的 Pi 可执行文件，并在缺失时避免创建不完整运行态。

#### 场景：sealed run 只要求实际自动阶段工具

- **给定** planning 配置 `cli: pi`
- **且** execution、review、archive 均未配置 Pi
- **当** 用户启动 sealed run
- **则** sealed run 工具发现不得要求 `pi` 存在

#### 场景：Pi 缺失时不创建 sealed run

- **给定** sealed run 自动阶段配置 `cli: pi`
- **且** PATH 中不存在 `pi`
- **当** 用户启动 sealed run
- **则** 系统返回找不到 `pi` 可执行文件的错误
- **且** 不创建用户状态目录中的 `runs/` 运行态文件

#### 场景：Pi 进程失败包含诊断

- **给定** Pi 进程返回非零 exit code
- **且** stderr 输出错误信息
- **当** `wo` 返回阶段失败
- **则** 错误信息包含 bounded stderr 诊断
- **且** run state 持久化为 failed

### 需求：动态审核修正迭代

系统必须将 `max_review_iterations = N` 解释为最多 N 轮 review/QA，每轮 review 或 QA 后最多执行一次 fix。

#### 场景：不执行审核

- **当** `max_review_iterations` 为 `0`
- **且** execution 阶段完成
- **则** 系统直接进入 archive 阶段

#### 场景：最后一轮审核仍需修正

- **当** `review_N.json` 的 `decision` 为 `needs_fix`
- **且** `N == max_review_iterations`
- **则** 系统进入 `fix_N`
- **且** 修正完成后将 run 状态标记为 `blocked_review_limit`
- **且** 系统不得进入 archive
- **且** 系统不得提交归档结果

### 需求：工作流上下文边界

系统必须把角色会话和可复核 artifact 视为不同职责的上下文来源。

#### 场景：角色会话承接连续推理

- **给定** sealed run 已完成 execution 并进入多轮 review/fix
- **当** 系统执行后续 `review_N` 或 `fix_N`
- **则** 系统继续按当前 agent tool 和角色复用已有 session id
- **且** review 阶段复用 reviewer 会话
- **且** fix 阶段复用 fixer 会话

#### 场景：prompt context 暴露角色会话状态

- **给定** 当前阶段为 `review_N`、`qa_N` 或 `fix_N`
- **当** 系统渲染该阶段 prompt
- **则** prompt context 必须包含 `RoleSessionKey`、`RoleSessionID`、`HasRoleSession` 和 `IsFirstRoleTurn`
- **且** `RoleSessionKey` 必须按当前阶段 effective tool 和当前角色生成，例如 `codex:reviewer`、`codex:qa` 或 `codex:fixer`
- **且** 只有同一 tool 和同一 role 的 session id 已存在时，`HasRoleSession` 才为 true，`IsFirstRoleTurn` 才为 false
- **且** 不得因为其他 tool 或其他 role 存在 session id 而误判为续轮

#### 场景：artifact 作为可复核事实来源

- **给定** sealed run 进入任一 agent 阶段
- **当** 系统渲染该阶段 prompt
- **则** prompt 必须指向当前 run 的 `state.json`
- **且** 需要阶段产物时必须指向当前阶段应写入或读取的 artifact
- **且** prompt 不得要求 agent 依赖会话记忆作为唯一事实来源

### 需求：命名 prompt 模板

系统必须从 YAML 的 `wo.prompts.planning/execution/review/qa/fix/archive` 读取 prompt，不再读取固定编号的 `1.md` 到 `9.md`，也不再读取 `.wo/cmd` 或 `~/.wo/cmd`。未知 prompt 键必须在配置读取阶段被拒绝。

#### 场景：review prompt 只默认暴露必要历史

- **给定** 当前阶段为 `review_5`
- **且** 前 4 轮 review/fix 已完成
- **当** 系统渲染默认 review prompt
- **则** prompt 必须包含当前 run 的 `state.json`
- **且** prompt 必须包含当前 change 路径、当前完整 diff 审核范围和 `review-5.json` 输出路径
- **且** prompt 必须包含上一轮 `review-4.json` 和 `fix-4-summary.md`
- **且** prompt 必须表达历史 review/fix 数量
- **且** prompt 不得默认列出 `review-1.json`、`review-2.json`、`review-3.json`
- **且** prompt 不得默认列出 `fix-1-summary.md`、`fix-2-summary.md`、`fix-3-summary.md`
- **且** prompt 必须说明旧历史只在重复 finding、证据矛盾、最新 artifact 引用旧 finding 或升级原因不清时按需追溯

#### 场景：execution 首轮提示词保留完整执行合同

- **给定** 当前阶段为 `execution`
- **当** 系统渲染默认 execution prompt
- **则** prompt 必须调用 `oz-exec` 技能并指向当前 run 的 `state.json`
- **且** prompt 必须要求读取 `proposal.md`、`design.md`、`spec.md`、`task.md`、`acceptance.json` 和 `tests/`
- **且** prompt 必须要求先运行 `acceptance.json` 中的 `required_tests[].command`
- **且** prompt 必须禁止删除、弱化、跳过或改写创建阶段契约测试和 `acceptance.json`
- **且** prompt 必须说明 execution 完成标准来自 `oz status <change> --json` 的 `tasks.total` 和 `tasks.done`
- **且** prompt 不得混入 review/fix 当前轮 artifact，例如 `review-1.json`、`fix-1-summary.md` 或“只修复当前 review/QA artifact”

#### 场景：review 续轮隐藏 JSON 示例

- **给定** 当前阶段为 `review_2`
- **且** 当前 tool 的 reviewer session id 已存在
- **当** 系统渲染默认 review prompt
- **则** prompt 必须包含当前 `review-2.json` 输出路径和上一轮 `review-1.json`、`fix-1-summary.md`
- **且** prompt 必须继续要求只输出一个 JSON 对象
- **且** prompt 不得包含 JSON 示例代码块、clean 示例、needs_fix 示例或 workflow_failure 示例正文

#### 场景：QA 续轮隐藏 JSON 示例

- **给定** 当前阶段为 `qa_2`
- **且** 当前 tool 的 QA session id 已存在
- **当** 系统渲染默认 QA prompt
- **则** prompt 必须包含当前 `qa-2.json` 输出路径
- **且** prompt 必须继续要求 `acceptance_matrix` 覆盖 `acceptance.json`
- **且** prompt 不得包含 clean 示例、needs_fix 示例或任何 JSON 示例代码块

#### 场景：fix prompt 聚焦当前 review

- **给定** 当前阶段为普通 `fix_2`
- **且** 当前 tool 的 fixer session id 已存在
- **当** 系统渲染默认 fix prompt
- **则** prompt 必须包含当前 run 的 `state.json` 和当前 `review-2.json`
- **且** prompt 必须要求只修复当前 review artifact 中列出的 findings
- **且** prompt 不得要求读取所有旧 review/fix artifact
- **且** prompt 不得重复首轮完整修复方法论正文

#### 场景：review 主动声明无法继续自动修复

- **给定** 当前阶段为第 2 轮及之后的 `review_N`
- **且** 审核智能体发现连续两轮没有实质变化
- **且** 最新 fix summary 表达无法完成或缺少继续修复条件
- **当** 审核智能体在 `review-N.json` 写入 `workflow_failure.failed: true`
- **则** 系统检测到该字段后必须将 run 置为 `failed`
- **且** 系统不得继续进入下一轮 review/fix 循环

#### 场景：升级 fix prompt 保留失败分析

- **给定** 当前阶段为已触发升级的 `fix_5`
- **当** 系统渲染默认 fix prompt
- **则** prompt 必须包含升级原因和重复 finding 标题
- **且** prompt 必须要求分析上一轮为什么没有解决问题
- **且** prompt 仍必须要求保持改动最小且聚焦当前 findings

#### 场景：读取阶段模板

- **当** 系统需要执行 review 阶段
- **则** 系统读取有效 YAML 配置中的 `prompts.review`
- **且** 使用当前轮次和 run 上下文渲染模板
- **且** 将渲染结果传给当前阶段 effective agent tool

#### 场景：模板缺失变量

- **当** prompt 模板引用不存在的变量
- **则** 系统报错
- **且** 不调用 agent tool 执行该阶段

#### 场景：sealed run 快照模板

- **当** sealed run 开始
- **则** 系统将 planning、execution、review、qa、fix、archive 的有效 prompt 快照到用户状态目录中的 `runs/<run-id>/prompt-snapshot.yaml`
- **且** `prompt-snapshot.yaml` 必须包含 `prompts.planning`、`prompts.execution`、`prompts.review`、`prompts.qa`、`prompts.fix` 和 `prompts.archive`
- **且** 新 run 不创建 `runs/<run-id>/prompts/` 目录
- **当** 用户恢复该 run
- **则** 系统优先使用 `prompt-snapshot.yaml` 中的 run 快照
- **且** 不重新读取当前 `wo.yaml`、`~/wo.yaml`、`.wo/cmd` 或 `~/.wo/cmd`
- **且** 当 `prompt-snapshot.yaml` 缺失时，系统必须报错而不是回退当前配置或历史 prompt 文件

#### 场景：历史 sealed run 旧写作阶段快照失败关闭

- **给定** `prompt-snapshot.yaml` 中存在 `prompts.writing`
- **当** 系统渲染 `execution` 阶段 prompt
- **则** 必须报错提示缺少当前 `prompts.execution` 快照
- **当** 系统渲染任意 `qa_N` 阶段 prompt
- **则** 必须报错提示缺少当前 `prompts.qa` 快照
- **当** 系统渲染任意 `fix_N` 阶段 prompt
- **则** 必须报错提示缺少当前 `prompts.fix` 快照

### 需求：严格 review artifact

系统必须要求 review artifact 同时包含决策、检查项、证据和 findings。

#### 场景：clean 审核

- **当** review artifact 的 `decision` 为 `clean`
- **则** `findings` 必须为空数组
- **且** 所有 `checks` 字段必须为 `true`
- **且** `evidence` 必须引用验证命令 artifact
- **且** `evidence` 必须引用截图、trace、QA、浏览器控制台、网络或等价运行时证据

#### 场景：needs_fix 审核

- **当** review artifact 的 `decision` 为 `needs_fix`
- **则** `findings` 必须至少包含一项
- **且** 每项 finding 必须包含非空 `title`、`severity`、`evidence` 和 `recommendation`

### 需求：严格 QA artifact

系统必须要求 QA artifact 用结构化矩阵证明 acceptance 合同已经满足。

#### 场景：clean QA

- **当** `qa_i.json` 的 `decision` 为 `clean`
- **则** `findings` 必须为空数组
- **且** `evidence` 必须包含可复核的测试、截图、trace、控制台、网络或运行时证据
- **且** `acceptance_matrix` 必须逐项覆盖 `acceptance.json.required_tests[].id`
- **且** `acceptance_matrix` 必须逐项覆盖 `acceptance.json.required_evidence[].id`
- **且** `acceptance_matrix[].id` 不得包含 acceptance 合同中未定义的 id
- **且** 每项矩阵记录的 `status` 必须为 `passed`

#### 场景：QA 证据缺项

- **给定** `acceptance.json` 要求 7 张截图或等价证据
- **当** `qa_i.json` 只引用其中一部分证据并声明 `clean`
- **则** 系统必须拒绝该 QA artifact
- **且** 不得进入 archive 阶段

#### 场景：QA 矩阵引用未定义 id 时打回同阶段

- **给定** `acceptance.json` 中不存在 `web-runtime-desktop-mobile-console-network`
- **当** `qa_i.json` 的 `acceptance_matrix[].id` 引用了该 id 并声明 `clean`
- **则** 系统必须记录同阶段 artifact gate 失败
- **且** 下一次运行必须重新进入同一个 `qa_i` 阶段
- **且** prompt 必须包含失败 artifact 路径和未定义 id 的错误摘要
- **且** 未达到最大尝试次数前不得把 run 或 batch 标记为 `failed`

### 需求：统一主阶段 artifact gate retry

系统必须在任意主阶段 agent 返回后检查该阶段应有产物。产物缺失、格式非法或合同不满足时，系统必须记录 `Stage artifact gate failed`，resume 同一角色 session，要求只补写或改写当前阶段产物，最多重试 3 次。第三次仍失败后才进入阻断状态。

#### 场景：所有主阶段产物缺失或非法都会同会话重试

- **给定** 默认 `go-dag` workflow 正在执行 execution、review、fix、QA 或 archive
- **当** 当前阶段 agent 返回后未完成该阶段产物，或写出的 artifact 不满足 schema / acceptance / readiness 合同
- **则** 系统必须记录同阶段 artifact gate 失败
- **且** 下一次运行必须 resume 同一角色 session
- **且** retry prompt 必须包含 `Stage artifact gate failed`、目标阶段、目标 artifact 路径和失败原因
- **且** retry prompt 必须要求只补写或改写当前阶段产物，不得修改 acceptance 合同或其他阶段 artifact
- **且** execution 阶段 task 未完成、fix summary 缺失、archive delivery summary 或归档目录缺失也必须进入同一 artifact gate retry
- **且** 未达到最大尝试次数前不得把 run 或 batch 标记为 `failed`

#### 场景：batch 中阶段产物修复后继续后续 change

- **给定** batch 中当前 change 的 execution 产物首次未完成
- **当** 同一 executor session 在 artifact gate retry 中修复该产物
- **则** batch worker 必须继续执行后续 change
- **且** batch state 不得因为可修复 artifact gate failure 进入 `failed`
- **且** 只有达到重试上限、真实 agent/backend 失败或不可恢复阻断时，batch 才能停止为 `failed`

### 需求：sealed run 配置快照

系统必须在 sealed run 开始时快照 effective workflow config，并在恢复时复用该快照。

#### 场景：恢复后配置文件已修改

- **当** sealed run 已创建
- **且** 用户修改仓库根目录 `wo.yaml`
- **且** 用户恢复未完成 run
- **则** 系统使用 `state.json` 中的 workflow config 快照
- **且** 不使用修改后的 `wo.yaml`

### 需求：已有配置和 run 快照兼容

系统必须保持已有用户配置和 sealed run 的兼容性。

#### 场景：已有用户自定义 prompt

- **给定** 仓库 `wo.yaml` 已配置自定义 `prompts.review`
- **当** 用户启动新的 run
- **则** 系统继续使用用户自定义 review prompt
- **且** 不自动用新的内置模板覆盖用户配置

#### 场景：恢复旧 run

- **给定** sealed run 已经创建并快照了旧 prompt
- **且** 程序升级后内置 prompt 已精简
- **当** 用户恢复该 run
- **则** 系统继续使用 run 目录中的 prompt 快照
- **且** 不重新读取新的内置默认 prompt

### 需求：Runner contract discovery

系统必须提供脚本友好的版本和能力发现接口，供下游 runner 在启动工作流前确认兼容性。

#### 场景：查询版本

- **当** 调用 `wo --version`
- **则** 命令 exit code 为 0
- **且** stdout 包含发布构建注入的 `v*` git tag 版本字符串
- **且** 本地源码构建未注入版本时，stdout 使用源码仓库的 `git describe --tags --always --dirty` 结果

#### 场景：查询 JSON contract

- **当** 调用 `wo contract --json`
- **则** 命令 exit code 为 0
- **且** stdout 是合法 JSON 对象
- **且** `json` 字段为 `true`
- **且** `capabilities` 是数组
- **且** `capabilities` 至少包含 `list-changes`、`run`、`resume`、`status`、`abort`

### 需求：JSON change listing

系统必须支持以 JSON 形式列出可运行的 active oz changes。

#### 场景：列出 active changes

- **当** 调用 `wo list-changes --json`
- **则** 命令 exit code 为 0
- **且** stdout 是合法 JSON 对象
- **且** `changes` 是数组
- **且** 每个 change 至少包含 `name`
- **且** 列表排除 `docs/changes/archive`、隐藏目录和不满足 change 验证规则的目录

### 需求：JSON sealed run start

系统必须支持下游 runner 以 JSON 模式启动 sealed run，并在 agent 阶段开始前尽快输出 run id。

#### 场景：启动 run

- **当** 调用 `wo run --change <change-name> --json`
- **且** `<change-name>` 是有效 active change
- **则** 命令作为长进程继续推进 sealed run
- **且** stdout 第一条以 `{` 开头的行是合法 JSON 对象
- **且** 该 JSON 包含 `run_id`
- **且** 该 JSON 不包含 `runId`
- **且** 该 JSON 包含 `changeName` 或 `change_name`
- **且** 该 JSON 的 `status` 为 `running`
- **且** 该 JSON 的 `stage` 为 `execution`
- **且** 输出该 JSON 前已创建 用户状态目录中的 `runs/<run-id>/state.json`

#### 场景：JSON 模式不输出人类 checklist

- **当** 使用 `wo run --change <change-name> --json`
- **则** stdout 不得输出交互菜单或 checklist 文本
- **且** 首行 JSON 后不得输出会破坏 JSONL 消费者的普通文本

### 需求：sealed run git baseline

系统必须在启动 sealed run 前确认当前 git 仓库存在可用的 `HEAD` commit，并只将可确认的无初始 commit 情况改写为可操作提示。

#### 场景：无提交仓库运行指定 change

- **给定** 当前目录位于一个已经 `git init` 但没有任何 commit 的仓库
- **且** 仓库中存在有效 active oz change
- **当** 用户运行 `wo --run <change-name>`
- **则** 命令必须失败
- **且** 错误信息必须明确说明需要先创建初始 git commit
- **且** 不得启动 execution agent 会话
- **且** 不得创建新的 用户状态目录中的 `runs/<run-id>/state.json`

#### 场景：无提交仓库使用 JSON runner 启动

- **给定** 当前目录位于一个已经 `git init` 但没有任何 commit 的仓库
- **且** 仓库中存在有效 active oz change
- **当** 调用 `wo run --change <change-name> --json`
- **则** 命令必须失败
- **且** 输出或错误必须包含可操作的无初始 commit 提示
- **且** 不得输出已创建的 running run state

#### 场景：已提交仓库启动 run

- **给定** 当前目录位于至少有一个 commit 的 git 仓库
- **且** 仓库中存在有效 active oz change
- **当** 用户运行 `wo --run <change-name>`
- **则** 系统按现有流程创建 sealed run
- **且** state 中记录 `baseline_head`
- **且** execution stage 可以继续发起 agent 会话

#### 场景：git 因其它原因失败

- **给定** git 命令因权限、安全目录、损坏仓库或其它非无提交原因失败
- **当** 用户启动 sealed run
- **则** 命令必须失败
- **且** 错误信息必须保留原始 git 诊断
- **且** 不得误导用户只需创建初始 commit

### 需求：JSON run resume

系统必须支持下游 runner 按指定 run id 恢复 sealed run。

#### 场景：恢复指定 run

- **当** 调用 `wo resume --run-id <run-id> --json`
- **且** 用户状态目录中的 `runs/<run-id>/state.json` 存在且可恢复
- **则** 命令作为长进程继续推进 sealed run
- **且** stdout 第一条以 `{` 开头的行是合法 JSON 对象
- **且** 该 JSON 包含 `runId` 或 `run_id`
- **且** 该 JSON 至少包含 `status` 和 `stage`
- **且** 恢复使用该 run 的 state 和 prompt/config 快照

### 需求：JSON run status

系统必须支持下游 runner 查询指定 run 的当前状态。

#### 场景：查询状态

- **当** 调用 `wo status --run-id <run-id> --json`
- **且** 用户状态目录中的 `runs/<run-id>/state.json` 存在
- **则** 命令 exit code 为 0
- **且** stdout 是合法 JSON 对象
- **且** JSON 至少包含 `run_id`、`status`、`stage`
- **且** JSON 不包含 `runId`
- **且** JSON 包含 `stages`、`paths`、`sessions` 和 `error` 字段

### 需求：统一 restart 命令

系统必须提供一个顶层 `wo restart` 命令，用于重启可恢复的单 run 或 batch。

#### 场景：默认重启最近可恢复任务

- **给定** 当前仓库存在一个最近失败的 batch
- **当** 用户运行 `wo restart`
- **则** 系统必须选择该 batch 作为重启目标
- **且** 启动 detached batch worker
- **且** 输出必须说明已重启该批量任务

#### 场景：默认重启最近可恢复单 run

- **给定** 当前仓库不存在可恢复 batch
- **且** 存在一个最近失败的单 run
- **当** 用户运行 `wo restart`
- **则** 系统必须选择该 run 作为重启目标
- **且** 启动 detached run worker
- **且** 输出必须说明已重启该工作流

#### 场景：没有可恢复任务

- **给定** 当前仓库只有 `done`、blocked 或 aborted 任务
- **当** 用户运行 `wo restart`
- **则** 系统必须返回明确错误
- **且** 不得创建 run
- **且** 不得修改已有状态文件

### 需求：restart 支持短编号指定目标

系统必须支持用现有 status 短编号指定 restart 目标。

#### 场景：重启指定 batch

- **给定** `wo status -b1` 能定位到一个 failed batch
- **当** 用户运行 `wo restart -b1`
- **则** 系统必须重启该 batch
- **且** 不得重启其他 batch 或单 run

#### 场景：重启指定单 run

- **给定** `wo status -w2` 能定位到一个 failed 单 run
- **当** 用户运行 `wo restart -w2`
- **则** 系统必须重启该 run
- **且** 不得重启 batch

#### 场景：短编号不存在

- **给定** 当前仓库不存在 `b99`
- **当** 用户运行 `wo restart -b99`
- **则** 系统必须返回明确错误
- **且** 不得修改任何任务状态

### 需求：单 run 重启复用原 sealed run

系统必须在重启单 run 时复用原 run，不得新建替代 run。

#### 场景：failed run 重启

- **给定** 单 run 状态为 `failed`
- **且** 当前 run 没有 active lock
- **当** 用户运行 `wo restart -w1`
- **则** 该 run 的 `status` 必须恢复为 `running`
- **且** `run_id` 必须保持不变
- **且** `change_name`、`workflow_config`、prompt 快照和 `sessions` 必须保持不变
- **且** 系统必须启动 detached run worker 继续该 run

#### 场景：interrupted run 重启

- **给定** 单 run 状态为 `interrupted`
- **当** 用户运行 `wo restart -w1`
- **则** 系统必须按原 stage 继续该 run
- **且** 不得重新创建该 run

#### 场景：running run 的 worker 已退出

- **给定** 单 run 状态仍为 `running`
- **且** run lock 不存在或已经 stale
- **当** 用户运行 `wo restart -w1`
- **则** 系统必须启动新的 detached run worker
- **且** 不得改变已完成阶段 artifact

### 需求：batch restart 删除失败记录并继续当前 change

系统必须在重启 failed batch 时停止复用普通失败 run，改为清除当前 change 的 failed run 关联并创建新 run 后继续队列。

#### 场景：failed batch 当前 run 可重新创建

- **给定** batch 状态为 `failed`
- **且** `failed_run_id` 对应 run 状态为 `failed`
- **且** 该 run 没有 active lock
- **当** 用户运行 `wo restart -b1`
- **则** batch 的 `status` 必须恢复为 `running`
- **且** batch 必须清理 `failed_change`、`failed_run_id` 和 `error`
- **且** batch 必须删除当前 change 在 `run_ids` 中的旧 run 关联
- **且** batch 的 `current_index` 必须保持不变
- **且** 系统必须启动 detached batch worker
- **且** batch worker 必须为当前 change 创建新的 run 后继续原队列

#### 场景：failed batch 当前 run 被 active lock 锁定

- **给定** batch 状态为 `failed`
- **且** 当前 failed run 存在 active lock
- **当** 用户运行 `wo restart -b1`
- **则** 系统必须返回锁冲突错误
- **且** 不得清理 batch 的 failed 字段
- **且** 不得删除 `run_ids` 中的旧 run 关联
- **且** 不得启动新的 batch worker

#### 场景：failed batch 尚未创建当前 run

- **给定** batch 状态为 `failed`
- **且** 当前 `changes[current_index]` 没有对应 run id
- **当** 用户运行 `wo restart -b1`
- **则** batch 必须恢复为 `running`
- **且** batch worker 必须为原当前 change 创建 run
- **且** 不得跳过该 change

#### 场景：缺失 run state 的 failed batch 拒绝重启

- **给定** batch 状态为 `failed`
- **且** `failed_run_id` 或当前 `run_ids` 中的 run id 非空
- **且** 对应 run state 文件缺失或不可读
- **当** 用户运行 `wo restart -b1`
- **则** 系统必须返回 "工作流记录缺失，无法自动确认恢复方式" 错误
- **且** 不得清理 batch 的 failed 字段
- **且** 不得删除 `run_ids` 中的旧 run 关联
- **且** 不得启动新的 batch worker

#### 场景：running batch 的 worker 已退出

- **给定** batch 状态为 `running`
- **且** 当前 run 没有 active lock
- **当** 用户运行 `wo restart -b1`
- **则** 系统必须启动新的 detached batch worker
- **且** 不得重排 `changes`
- **且** 不得修改 `current_index`

### 需求：不可自动恢复状态必须拒绝 restart

系统必须明确拒绝不应自动重启的终态或阻塞态。

#### 场景：blocked run 拒绝重启

- **给定** run 状态为 `blocked_review_limit` 或 `blocked_validation_limit`
- **当** 用户运行 `wo restart -w1`
- **则** 系统必须返回明确错误
- **且** 不得把 run 改回 `running`

#### 场景：blocked batch 拒绝重启

- **给定** batch 状态为 `failed`
- **且** 当前 failed run 状态为 `blocked_review_limit` 或 `blocked_validation_limit`
- **当** 用户运行 `wo restart -b1`
- **则** 系统必须返回明确错误
- **且** 不得把 batch 改回 `running`

#### 场景：不可自动恢复 run 拒绝 batch restart

- **给定** batch 状态为 `failed`
- **且** 当前 failed run 状态为 `blocked_review_limit`、`blocked_validation_limit` 或 `aborted_manual_intervention`
- **当** 用户运行 `wo restart -b1`
- **则** 系统必须返回明确错误
- **且** 不得把 batch 改回 `running`
- **且** 不得删除旧 run 关联

#### 场景：已完成或手动中止任务拒绝重启

- **给定** 任务状态为 `done`、`aborted_manual_intervention` 或 batch `aborted`
- **当** 用户运行 `wo restart`
- **则** 系统必须返回明确错误
- **且** 不得启动 worker

### 需求：restart 不得破坏并发安全

系统必须在重启前检查 live lock，避免重复 worker 同时推进同一任务。

#### 场景：active lock 拒绝重启

- **给定** 目标 run 存在 active lock
- **当** 用户运行 `wo restart -w1`
- **则** 系统必须返回锁冲突错误
- **且** 不得启动新的 detached worker

#### 场景：batch 当前 run 被 active lock 锁定

- **给定** 目标 batch 当前 run 存在 active lock
- **当** 用户运行 `wo restart -b1`
- **则** 系统必须返回锁冲突错误
- **且** 不得启动新的 batch worker

### 需求：JSON restart 入口稳定

系统必须为自动化调用提供明确 id 的 JSON restart 入口。

#### 场景：JSON 重启单 run

- **当** 调用 `wo restart --run-id <run-id> --json`
- **则** 系统必须只重启该 run
- **且** stdout 必须保持 RunnerState JSON 结构
- **且** 不得根据最近任务隐式选择其他目标

#### 场景：JSON 重启 batch

- **当** 调用 `wo restart --batch-id <batch-id> --json`
- **则** 系统必须只重启该 batch
- **且** 不得根据最近任务隐式选择其他目标

#### 场景：JSON 缺少明确 id

- **当** 调用 `wo restart --json`
- **则** 系统必须返回用法错误
- **且** 不得修改任何任务状态

### 需求：watch 默认观察正在运行的任务

系统必须提供 `wo watch` 命令持续刷新任务状态，并优先展示正在运行的 batch。

#### 场景：running batch 优先于 single-run

- **给定** 当前仓库同时存在 running batch 和 running single-run
- **当** 用户运行 `wo watch`
- **则** stdout 必须展示 running batch 的状态
- **且** stdout 不得默认展示 single-run
- **且** 命令必须持续刷新直到收到 Ctrl-C

#### 场景：没有 running batch 时回退 single-run

- **给定** 当前仓库不存在 running batch
- **且** 当前仓库存在 running single-run
- **当** 用户运行 `wo watch`
- **则** stdout 必须展示该 single-run 的状态
- **且** 命令必须持续刷新直到收到 Ctrl-C

#### 场景：没有运行任务时提示空状态

- **给定** 当前仓库没有 running batch
- **且** 当前仓库没有 running single-run
- **当** 用户运行 `wo watch`
- **则** stdout 必须提示当前没有正在进行的批量任务或工作流
- **且** 命令必须正常退出或在下一次刷新前不修改任何 state

### 需求：watch 支持指定目标和 spinner

系统必须支持用短编号指定 watch 目标，并在 running 阶段展示旋转 indicator。

#### 场景：watch 指定 batch

- **给定** `wo status -b1` 能定位一个 batch
- **当** 用户运行 `wo watch -b1`
- **则** stdout 必须持续展示该 batch 的状态
- **且** 不得因为存在更新的 single-run 而切换目标

#### 场景：watch 指定 single-run

- **给定** `wo status -w1` 能定位一个 single-run
- **当** 用户运行 `wo watch -w1`
- **则** stdout 必须持续展示该 workflow 的状态
- **且** 不得因为存在 running batch 而切换目标

#### 场景：running 阶段显示 spinner

- **给定** watch 目标中存在当前 running 阶段
- **当** 用户运行 `wo watch`
- **则** 当前 running 阶段必须显示 `|`、`/`、`-`、`\` 中的某个 spinner 帧
- **且** 后续刷新必须能切换 spinner 帧
- **且** `wo status` 的普通输出仍可继续使用静态 running 标记

### 需求：人类 run status 极简固定列视图

系统必须支持用户通过 `wo status` 和 `wo watch` 查看同一套极简固定列进度视图，而不是内部 workflow stage 列表、标题摘要或 engine 诊断行。

#### 场景：查询人类状态

- **给定** 当前 run 已完成 planning，正在 execution，且配置了 implementation context 子代理
- **当** 调用 `wo status -w1`
- **则** stdout 第一行必须是静态 indicator 和 workflow 短编号，例如 `→ w1`
- **且** 主阶段行必须按 `阶段中文名 session-id marker 耗时分钟` 四列输出，例如 `规划阶段 planner-session ✓ 2.00` 和 `执行阶段 writer-session → 6.50`
- **且** 子代理行必须缩进两个空格并使用用户可读短名，例如 `  代码侦察 explore-session ✓ 1.50`
- **且** marker 只使用 `-`、`→`、`✓` 或 `x`
- **且** 耗时必须格式化为两位小数分钟，不追加单位
- **且** 输出不得包含 `工作流`、状态英文单词、change name、`引擎`、并行 group 汇总行或总耗时行

#### 场景：规划会话可见

- **给定** 当前 run 的 sessions 中记录了 planning 会话 id
- **当** 调用 `wo status -w1`
- **则** stdout 包含 `规划阶段` 行
- **且** `规划阶段` 行显示 planning 会话 id
- **且** `规划阶段` 行排在执行、审核、修正、测试和归档阶段之前

#### 场景：归档会话独立展示

- **给定** 当前 run 已进入 archive 阶段
- **当** 调用 `wo status -w1`
- **则** stdout 包含 `归档阶段` 行
- **且** `归档阶段` 行显示 archiver session id
- **且** `归档阶段` 行不得复用 executor session id

#### 场景：当前正在修复

- **给定** run 当前 stage 为 `fix_2`
- **且** execution 和 fix_1 已完成
- **当** 调用 `wo status -w1`
- **则** `执行阶段` 行显示 executor session id 和 `✓`
- **且** `修正阶段` 行显示 fixer session id 和 `→`
- **且** `执行阶段` 行不得显示 `→`
- **且** `审核阶段` 行不得显示 `→`

#### 场景：多轮修复只展示一条 fixer 行

- **给定** run 已完成 `fix_1` 和 `fix_2`
- **且** `state.json.sessions` 存在 `<tool>:fixer`
- **当** 调用 `wo status -w1`
- **则** stdout 必须只包含一条聚合后的 `修正阶段` 行
- **且** `修正阶段` 行显示 `<tool>:fixer` 对应 session id
- **且** `修正阶段` 行显示 `✓`

#### 场景：历史修复缺少 fixer session

- **给定** 历史 run 的 stages 包含已完成的 `fix_1`
- **且** `state.json.sessions` 没有任何 `<tool>:fixer`
- **且** `state.json.sessions` 存在 `<tool>:executor`
- **当** 调用 `wo status -w1`
- **则** `修正阶段` 行必须显示 `未知`
- **且** `修正阶段` 行不得展示 executor session id

#### 场景：当前正在审核

- **给定** run 当前 stage 为 `review_3`
- **且** review_1 和 review_2 已完成
- **当** 调用 `wo status -w1`
- **则** `审核阶段` 行显示 reviewer session id
- **且** `审核阶段` 行显示 `→`
- **且** `执行阶段` 行不得显示 `→`
- **且** `修正阶段` 行不得显示 `→`

#### 场景：已发生阶段缺少会话 id

- **给定** run id 为 `20260512T051106.925886354Z`
- **且** run 已进入或完成 execution 阶段
- **且** run 状态中没有 executor session id
- **当** 调用 `wo status -w1`
- **则** `执行阶段` 行必须显示 `未知`
- **且** `执行阶段` 行不得包含 run id `20260512T051106.925886354Z`

#### 场景：归档完成

- **给定** run 已完成 archive 阶段
- **当** 调用 `wo status -w1`
- **则** stdout 包含 `归档阶段` 行
- **且** `归档阶段` 行显示 archiver session id
- **且** `归档阶段` 行显示 `✓`

### 需求：status 默认目标和短编号

系统必须让无参数 `wo status` 默认展示当前仓库最新 batch；当前仓库没有 batch 时，回退展示最新 workflow。系统必须支持 `wo status -bN` 和 `wo status -wN` 查询当前仓库内按新到旧排序的第 N 个 batch 或 workflow。

#### 场景：默认查看最新 batch

- **给定** 当前仓库存在 batch state
- **当** 用户运行 `wo status`
- **则** stdout 第一行必须显示 indicator、batch 短编号和队列进度，例如 `→ b1 1/2`
- **且** batch 标题行不得包含最新 batch 的真实 batch id

#### 场景：查询历史 batch 或 workflow

- **给定** 当前仓库存在至少两个 batch 和两个 workflow
- **当** 用户运行 `wo status -b2`
- **则** 输出必须展示倒数第二个 batch
- **当** 用户运行 `wo status -w2`
- **则** 输出必须展示倒数第二个 workflow 的阶段进度
- **且** 单 workflow 查询不得切换到 batch 队列视图

#### 场景：短编号不存在

- **给定** 当前仓库只有一个 batch 或 workflow
- **当** 用户运行 `wo status -b2` 或 `wo status -w2`
- **则** 命令必须失败
- **且** 错误必须说明找不到对应短编号

### 需求：batch 状态人类输出

系统必须在 `wo status` 的人类可读输出中展示 batch 级别的整体状态，让用户能看到队列总进度和每个 change，并展开 batch 内所有已创建 workflow 的极简固定列视图。

#### 场景：运行中的 batch 展示整体和当前工作流

- **给定** 当前仓库存在一个运行中的 batch
- **且** batch 包含 `1-a`、`2-b` 和 `3-c`
- **且** `1-a` 对应的 run 已完成
- **且** `2-b` 对应的 run 正在执行 `review_1`
- **且** `3-c` 尚未创建 run
- **当** 用户运行 `wo status`
- **则** 第一行必须显示 indicator、batch 短编号和整体进度，例如 `→ b1 2/3`
- **且** 批量任务组名不得包含真实 batch id
- **且** 输出必须显示整体进度为 `2/3`
- **且** 输出必须把 `1-a`、`2-b` 和 `3-c` 分别显示为只包含 change 名称的独立行
- **且** change 行不得包含 workflow 短编号、run id、索引、状态或运行中标记
- **且** 未开始的 `3-c` 行不得追加 `未开始`
- **且** 每个已创建 workflow 行下方必须紧跟它自己的 `→ wN` header 和固定列阶段/子代理行
- **且** 未开始 change 下方不得显示伪造的内部阶段

#### 场景：batch 刚提交但尚未创建 run

- **给定** 当前仓库存在一个运行中的 batch state
- **且** batch 的 `run_ids` 为空
- **当** 用户运行 `wo status`
- **则** 输出必须包含 batch 短编号、状态和整体进度
- **且** 输出必须列出 batch 中所有 change
- **且** 每个 change 行不得显示为 `未开始`
- **且** 命令不得因为缺少当前 run 而返回“没有 wo run”

#### 场景：batch 已完成

- **给定** 当前仓库最新 run 属于一个已完成 batch
- **且** batch 中所有 change 都有对应 run id
- **当** 用户运行 `wo status`
- **则** 输出必须包含 batch 短编号、状态和整体进度
- **且** batch 状态必须显示为 `done`
- **且** 每个队列内 change 行不得包含对应 run id
- **且** 每个已完成 change 的阶段明细必须显示完成标记

#### 场景：当前 run 失败后 batch 停止

- **给定** batch 包含 `1-a` 和 `2-b`
- **且** `1-a` 对应 run 状态为 `failed`
- **且** batch state 记录 `failed_change` 为 `1-a`
- **且** batch state 记录 `failed_run_id` 为该 run id
- **当** 用户运行 `wo status`
- **则** 输出必须包含失败 change `1-a`
- **且** change 行不得包含失败 run id
- **且** 输出必须包含脱敏后的 batch 失败摘要
- **且** `1-a` 下方的阶段明细必须体现失败状态
- **且** `2-b` 必须只显示 change 名称，不追加 `未开始`

#### 场景：failed batch 不暴露内部网络诊断

- **给定** 当前仓库最新 batch 状态为 `failed`
- **且** batch state 的 `error` 包含 `stderr`、`backend-api`、`wss://chatgpt.com`、`websocket` 或 `tls handshake eof`
- **当** 用户运行 `wo status`
- **则** stdout 必须包含 `批量任务 b1 failed`
- **且** stdout 必须包含失败 change 名称
- **且** stdout 必须包含简短失败摘要
- **且** stdout 不得包含 `backend-api`
- **且** stdout 不得包含 `wss://chatgpt.com`
- **且** stdout 不得包含 `tls handshake eof`
- **且** stdout 不得包含 `websocket`
- **且** stdout 不得包含 raw `stderr` 诊断正文

#### 场景：failed batch 使用稳定失败摘要

- **给定** batch state 记录 `failed_run_id`
- **且** 对应 run 的 `status` 为 `failed`
- **且** 对应 run 的 `error` 包含多行底层进程错误
- **当** 用户运行 `wo status`
- **则** batch 错误行必须显示稳定摘要 `failed`
- **且** batch 错误行不得显示 run 的 raw error

#### 场景：当前 run 达到审核或校验阻塞

- **给定** batch 中当前 run 状态为 `blocked_review_limit` 或 `blocked_validation_limit`
- **当** 用户运行 `wo status`
- **则** 输出必须把该 run 标记为失败或阻塞
- **且** 输出必须包含具体阻塞状态枚举
- **且** 后续未启动 change 不得显示 run id

#### 场景：当前仓库只有单个 sealed run

- **给定** 当前仓库没有 batch state
- **且** 最新 run 不包含 `batch_id`
- **当** 用户运行 `wo status`
- **则** 输出必须继续使用单 workflow 极简固定列视图
- **且** 输出不得出现 `批量任务`
- **且** 输出不得出现 batch 队列行

#### 场景：查询 batch 内 run 的 JSON status

- **给定** 一个属于 batch 的 run
- **当** 调用 `wo status --run-id <run-id> --json`
- **则** stdout 必须仍是可解析 JSON
- **且** JSON 字段名仍包含 `run_id`、`change_name`、`status`、`stage`、`stages`、`paths`、`sessions` 和 `error`
- **且** JSON 不得包含 `批量任务`、`工作流`、完成标记或运行中标记
- **且** `stage` 必须仍使用内部 stage 值，例如 `execution`、`review_1`、`fix_1` 或 `archive`

### 需求：单 workflow status 展示阶段耗时

系统必须在 `wo status -wN` 的人类可读输出中把已实际执行阶段的耗时展示在对应固定列行中，不再输出总耗时公式或独立耗时块。

#### 场景：完成的 workflow 展示分钟级耗时

- **给定** 当前仓库存在一个已完成 workflow
- **且** 该 workflow 的 `execution`、`review_1`、`archive` 阶段都有开始和结束时间
- **当** 用户运行 `wo status -w1`
- **则** `执行阶段`、`审核阶段` 和 `归档阶段` 行必须分别在第四列显示两位小数分钟
- **且** 输出不得包含 `耗时`、`分钟` 或总耗时公式

### 需求：跳过阶段不进入耗时统计

系统必须只统计实际执行过并有有效 timing 的阶段，不得把 artifact skip 或旧状态中只有 completed 标记的阶段写入耗时公式。

#### 场景：completed 但没有 timing 的阶段被跳过

- **给定** 当前仓库存在一个 workflow
- **且** `review_1` 在 `stages` 中为 `completed`
- **但** `review_1` 没有 `stage_timings` 记录
- **当** 用户运行 `wo status -w2`
- **则** 对应阶段行不得把缺失 timing 的 `review_1` 写入耗时
- **且** 输出不得包含独立耗时拆分块

### 需求：batch status 在对应 change 下展示耗时

系统必须在 batch 人类输出中把每个已创建 run 的固定列阶段耗时展示在对应 change 下方，未开始 change 不显示伪造耗时。

#### 场景：batch 中只有已创建 run 显示耗时

- **给定** 当前仓库存在一个包含两个 change 的 batch
- **且** 第一个 change 已创建 run 并有阶段 timing
- **且** 第二个 change 尚未开始
- **当** 用户运行 `wo status`
- **则** 第一个 change 下必须包含缩进的 workflow header 和阶段耗时列
- **且** 第二个 change 下不得出现阶段耗时行

### 需求：JSON status 机器接口保持兼容

系统必须保持 `wo status --run-id <run-id> --json` 的 runner 顶层 contract 不变，同时允许新增 `observability` 字段暴露阶段和子代理产物路径。耗时统计只出现在人类可读输出。

#### 场景：JSON 输出不包含耗时内部字段

- **给定** 当前仓库存在一个带 `stage_timings` 的 run
- **当** 用户运行 `wo status --run-id <run-id> --json`
- **则** 输出必须是合法 JSON
- **且** JSON 必须包含既有字段 `run_id`、`change_name`、`status`、`stage`、`stages`、`paths`、`sessions` 和 `error`
- **且** JSON 可以新增 `observability.engine`、`observability.rows` 和 `observability.artifacts`
- **且** JSON 不得包含 `stage_timings`
- **且** JSON 不得包含 `耗时` 或 `分钟`

#### 场景：JSON observability 暴露固定产物路径

- **给定** 当前仓库存在一个 sealed run
- **当** 用户运行 `wo status --run-id <run-id> --json`
- **则** `observability.rows` 必须为 execution、review、fix、qa 和 archive 阶段给出稳定行
- **且** 每个阶段 row 必须包含阶段中文名、session id、marker 和固定 `stage_artifact` 路径
- **且** 即使审核、测试或归档尚未开始，也必须给出 `review-1.json`、`qa-1.json` 和 `delivery-summary.md` 的预期路径
- **且** subagent row 必须给出短名、完整名称、session id、`member_artifact` 和 `group_artifact` 绝对路径

### 需求：失败 batch 展示可理解原因

系统必须在 batch 人类可读状态中展示失败 change、失败阶段和中文失败摘要，不能只显示 `failed`。

#### 场景：普通 failed batch 显示失败上下文

- **给定** 当前仓库最新 batch 状态为 `failed`
- **且** batch 记录了 `failed_change` 和 `failed_run_id`
- **且** 对应 run 状态为 `failed`
- **当** 用户运行 `wo status`
- **则** stdout 必须包含 `批量任务 b1 failed`
- **且** stdout 必须包含失败 change 名称
- **且** stdout 必须包含失败阶段对应的中文角色
- **且** stdout 必须包含中文失败摘要
- **且** stdout 不得只用 `错误: failed` 表达失败原因

#### 场景：内部网络错误不外泄 raw 诊断

- **给定** failed run 的 error 包含 `stderr`、`backend-api`、`wss://chatgpt.com`、`websocket` 或 `tls handshake eof`
- **当** 用户运行 `wo status`
- **则** stdout 必须包含可理解的中文失败摘要
- **且** stdout 不得包含 `backend-api`
- **且** stdout 不得包含 `wss://chatgpt.com`
- **且** stdout 不得包含 `websocket`
- **且** stdout 不得包含 raw `stderr` 诊断正文

#### 场景：阻塞状态保留业务原因

- **给定** failed batch 的当前 run 状态为 `blocked_review_limit` 或 `blocked_validation_limit`
- **当** 用户运行 `wo status`
- **则** stdout 必须包含对应阻塞原因
- **且** stdout 不得把该状态泛化成普通智能体失败

#### 场景：缺失 run state 展示记录缺失原因

- **给定** failed batch 的 `failed_run_id` 或当前 run id 指向不存在的 run state 文件
- **当** 用户运行 `wo status`
- **则** stdout 必须展示 "工作流记录缺失，无法自动确认恢复方式"
- **且** stdout 必须包含整条 batch 历史的清理命令
- **且** stdout 不得提示 `wo restart -bN` 能自动继续

### 需求：可恢复 batch 优先提示 restart

系统必须把可恢复 batch 的首选操作提示为 `wo restart -bN`，而不是删除整个状态目录。

#### 场景：普通 failed batch 提示删除失败记录并继续

- **给定** 当前 batch 状态为 `failed`
- **且** 当前 failed run 状态为 `failed` 或 `interrupted`
- **当** 用户运行 `wo status`
- **则** stdout 必须提示用户运行 `wo restart -b1`
- **且** 提示语必须说明会删除失败记录并继续该批量任务
- **且** stdout 不得把 `rm -rf` 作为首选操作

#### 场景：启动时 stopped history 使用同一恢复提示

- **给定** 当前仓库存在 stopped failed batch
- **当** 用户无参数运行 `wo`
- **则** stdout 必须展示该 batch 的失败摘要
- **且** stdout 必须提示 `wo restart -bN` 删除失败记录并继续

### 需求：clean 只清理当前项目运行态

系统必须提供 `wo clean` 命令，只清理当前 git 仓库对应 repo-key 下的失败或异常运行态。

#### 场景：清理当前项目失败运行态

- **给定** 当前仓库的用户状态目录下存在 `failed` run
- **当** 用户在该仓库运行 `wo clean`
- **则** 系统必须删除该 run 目录
- **且** stdout 必须展示已清理工作流数量
- **且** stdout 必须说明该操作只删除 wo 历史记录，不回滚代码改动

#### 场景：不影响其他项目运行态

- **给定** 两个不同 git 仓库共享同一个 `XDG_STATE_HOME`
- **且** 两个仓库都存在失败 run
- **当** 用户在第一个仓库运行 `wo clean`
- **则** 系统必须只删除第一个仓库 repo-key 下的失败 run
- **且** 第二个仓库 repo-key 下的失败 run 必须仍然存在

#### 场景：无可清理对象时输出空状态

- **给定** 当前仓库没有失败或异常运行态
- **当** 用户运行 `wo clean`
- **则** stdout 必须提示没有需要清理的失败或异常运行态
- **且** 命令必须成功退出

### 需求：clean 保留可用历史和运行中任务

系统必须保留已完成历史，并跳过 active lock 保护的运行中任务。

#### 场景：保留已完成 run

- **给定** 当前仓库存在状态为 `done` 的 run
- **当** 用户运行 `wo clean`
- **则** 系统不得删除该 run 目录

#### 场景：保留已归档 run

- **给定** 当前仓库存在状态为 `archived_superseded` 的 run
- **当** 用户运行 `wo clean`
- **则** 系统不得删除该 run 目录

#### 场景：跳过 active running run

- **给定** 当前仓库存在状态为 `running` 的 run
- **且** 该 run 存在 active lock
- **当** 用户运行 `wo clean`
- **则** 系统不得删除该 run 目录
- **且** stdout 必须提示已跳过仍在运行的任务

### 需求：clean 清理异常 batch 及引用 run

系统必须把 failed/aborted batch 视为一条异常历史，清理 batch 时同步清理它引用的 run。

#### 场景：清理 failed batch 和失败 run

- **给定** 当前仓库存在状态为 `failed` 的 batch
- **且** batch 的 `failed_run_id` 指向一个 run
- **且** batch 的 `run_ids` 也包含该 run
- **当** 用户运行 `wo clean`
- **则** 系统必须删除该 batch 目录
- **且** 系统必须删除该 batch 引用的 run 目录
- **且** stdout 必须展示已清理批量任务数量和工作流数量

#### 场景：清理 aborted batch

- **给定** 当前仓库存在状态为 `aborted` 的 batch
- **且** batch 引用了一个已中止 run
- **当** 用户运行 `wo clean`
- **则** 系统必须删除该 batch 目录
- **且** 系统必须删除该 batch 引用的 run 目录

#### 场景：active run 保护整个 batch

- **给定** 当前仓库存在状态为 `failed` 的 batch
- **且** batch 引用的某个 run 存在 active lock
- **当** 用户运行 `wo clean`
- **则** 系统不得删除该 batch 目录
- **且** 系统不得删除该 active run 目录
- **且** stdout 必须提示已跳过仍在运行的任务

### 需求：clean 处理损坏运行态

系统必须能清理 state 缺失或 JSON 损坏的运行态目录，避免用户被坏历史卡住。

#### 场景：清理缺失 state.json 的 run 目录

- **给定** 当前仓库 `runs/<run-id>/` 目录存在
- **且** 该目录缺少 `state.json`
- **当** 用户运行 `wo clean`
- **则** 系统必须删除该 run 目录

#### 场景：清理 JSON 损坏的 batch 目录

- **给定** 当前仓库 `batches/<batch-id>/state.json` 存在
- **且** 文件内容不是合法 JSON
- **当** 用户运行 `wo clean`
- **则** 系统必须删除该 batch 目录

### 需求：清理异常 run 时同步清理 Codex/Pi 子会话记录

系统必须在 `wo clean` 删除失败、阻塞、中断、人工中止或损坏的 run 时，删除该 run 在 `state.json.sessions` 中引用的 Codex/Pi 子会话记录。

#### 场景：失败 run 的 Codex/Pi JSONL 和 Pi SQLite 行被清理

- **给定** 当前仓库存在一个状态为 `failed` 的 run
- **且** 该 run 的 `state.json.sessions` 包含 `codex:executor` 和 `pi:archiver`
- **且** Codex sessions 目录中存在匹配 Codex session id 的 JSONL 文件
- **且** Pi sessions 目录中存在匹配 Pi session id 的 JSONL 文件
- **且** Pi SQLite 数据库中存在匹配 Pi session id 的 session/message 行
- **当** 用户运行 `wo clean`
- **则** 该 run 目录必须被删除
- **且** 匹配的 Codex JSONL 文件必须被删除
- **且** 匹配的 Pi JSONL 文件必须被删除
- **且** Pi SQLite 中匹配该 Pi session id 的行必须被删除
- **且** 不匹配的其他 agent session 文件和 SQLite 行必须保留

### 需求：保留受保护 run 的外部 agent 记录

系统必须保留未被 `wo clean` 删除的 run 及其外部 Codex/Pi 子会话记录。

#### 场景：done 和 active-locked run 的 agent 记录保留

- **给定** 当前仓库存在一个状态为 `done` 的 run
- **且** 当前仓库存在一个带 active lock 的异常 run
- **且** 这两个 run 都引用 Codex/Pi session id
- **且** 外部 Codex/Pi JSONL 和 Pi SQLite 行存在
- **当** 用户运行 `wo clean`
- **则** done run 目录必须保留
- **且** active-locked run 目录必须保留
- **且** 这两个 run 引用的 Codex/Pi JSONL 文件必须保留
- **且** 这两个 run 引用的 Pi SQLite 行必须保留

### 需求：batch 清理同步处理被删除 run 的 agent 记录

系统必须在清理 failed 或 aborted batch 时，同步清理该 batch 引用且实际被删除的 run 的外部 agent 记录。

#### 场景：failed batch 引用的失败 run 清理 agent 记录

- **给定** 当前仓库存在一个状态为 `failed` 的 batch
- **且** batch 的 `run_ids` 或 `failed_run_id` 引用一个状态为 `failed` 的 run
- **且** 该 run 引用 Codex/Pi session id
- **当** 用户运行 `wo clean`
- **则** failed batch 必须被删除
- **且** 被引用的 failed run 必须被删除
- **且** 该 run 引用的 Codex/Pi 外部记录必须被删除

### 需求：外部存储缺失或 schema 未知不阻断 wo clean

系统必须把 Codex/Pi 外部存储清理作为附加清理，不得因为外部文件不存在或 Pi SQLite schema 无法识别而阻断 `wo` 自己的运行态清理。

#### 场景：缺失外部记录时仍清理 wo 状态

- **给定** 当前仓库存在一个状态为 `failed` 的 run
- **且** 该 run 记录了 Codex/Pi session id
- **但** 本机不存在匹配的 Codex/Pi JSONL 文件
- **且** Pi SQLite 不存在或 schema 无法识别
- **当** 用户运行 `wo clean`
- **则** 该 run 目录仍必须被删除
- **且** `wo clean` 必须返回成功

### 需求：输出展示 agent 子会话记录清理结果

系统必须在 `wo clean` 的人类可读输出中展示成功清理的 agent 子会话记录数量。

#### 场景：清理输出包含 agent 子会话记录数量

- **给定** 当前仓库存在一个可清理 run
- **且** 该 run 引用的 Codex/Pi 外部记录存在并被清理
- **当** 用户运行 `wo clean`
- **则** 输出必须包含清理的 workflow 数量
- **且** 输出必须包含清理的 agent 子会话记录数量
- **且** 输出必须继续说明该操作不回滚代码改动

### 需求：不可恢复提示使用 clean 命令

系统在不可恢复任务的人类输出中必须优先提示 `wo clean`，避免让用户手工复制内部 `rm -rf` 路径。

#### 场景：status 中 blocked batch 提示 clean

- **给定** 当前仓库存在不可自动恢复的 blocked batch
- **当** 用户运行 `wo status -b1`
- **则** stdout 必须提示可运行 `wo clean` 清理失败或异常运行态
- **且** stdout 必须说明该操作不回滚代码改动
- **且** stdout 不得把裸 `rm -rf` 作为首选清理命令

#### 场景：help 展示 clean 命令

- **当** 用户运行 `wo --help`
- **则** stdout 必须包含 `wo clean`
- **且** 命令说明必须表达清理当前项目失败或异常运行态

### 需求：不可恢复任务提供清理历史命令

系统必须只在无法自动恢复时展示整条历史的删除命令，并说明该命令只清理 wo 历史记录。

#### 场景：aborted batch 提示清理整条历史

- **给定** 当前 batch 状态为 `aborted`
- **当** 用户运行 `wo status -b1`
- **则** stdout 必须包含用户已中止的原因
- **且** stdout 必须包含 `rm -rf`
- **且** 清理路径必须指向当前仓库用户状态目录下的 `batches/<batch-id>`
- **且** stdout 必须说明清理不会回滚代码改动

#### 场景：blocked batch 不提示自动恢复

- **给定** 当前 batch 状态为 `failed`
- **且** 当前 failed run 状态为 `blocked_review_limit` 或 `blocked_validation_limit`
- **当** 用户运行 `wo status -b1`
- **则** stdout 不得提示 `wo restart -b1` 能自动继续
- **且** stdout 必须包含整条 batch 历史的清理命令

### 需求：机器接口保持兼容

系统必须只改变人类可读输出和 restart 行为，不改变现有 JSON status contract。

#### 场景：JSON status 不包含人类提示

- **给定** 一个 failed run
- **当** 用户运行 `wo status --run-id <run-id> --json`
- **则** stdout 必须是可解析 JSON
- **且** JSON 字段名必须仍包含 `run_id`、`change_name`、`status`、`stage`、`stages`、`paths`、`sessions` 和 `error`
- **且** JSON 不得包含 `wo restart`
- **且** JSON 不得包含 `rm -rf`
- **且** JSON 不得包含 watch spinner

### 需求：启动提示结构化展示已停止任务

系统必须在无参数启动 `wo` 时，用结构化多行提示展示失败或已停止的历史 batch/workflow，避免重复和难读的一行输出。

#### 场景：失败 batch 展示停止位置和原因

- **给定** 当前仓库存在状态为 `failed` 的 batch
- **且** batch state 记录 `failed_change`
- **且** batch state 记录 `failed_run_id`
- **当** 用户无参数运行 `wo`
- **则** 输出必须包含“检测到已停止的历史任务”
- **且** 输出必须以批量任务条目展示 batch 短编号、真实 batch id 和 batch 状态
- **且** 输出必须单独展示 failed change
- **且** 输出必须单独展示 failed run 的短编号和真实 run id
- **且** 输出必须展示简洁 reason，例如 `blocked_review_limit`

#### 场景：batch 内失败 run 不重复展示为独立 workflow

- **给定** 一个失败 batch 的 `failed_run_id` 指向某个 run
- **且** 该 run 本身也是失败或阻塞状态
- **当** 用户无参数运行 `wo`
- **则** 该 run 必须只出现在失败 batch 条目中
- **且** 不得再出现在独立失败 workflow 条目中

#### 场景：独立失败 workflow 与失败 batch 并存

- **给定** 当前仓库同时存在一个失败 batch
- **且** 当前仓库存在一个不属于 batch 的失败 run
- **当** 用户无参数运行 `wo`
- **则** 输出必须在同一个已停止历史任务提示下展示 batch 条目
- **且** 输出必须展示独立 workflow 条目
- **且** 两类条目必须能通过“批量任务”和“工作流”区分

### 需求：status 人类输出提示可用更新

系统必须在人类可读 `wo status` 中使用用户级缓存检查 `wo` 和 `oz` 是否存在 GitHub 新版本，并在存在更新时提示用户运行 `wo update`。

#### 场景：缓存显示有更新

- **给定** 更新检查缓存未过期
- **且** 缓存记录 `wo` 或 `oz` 存在新版本
- **当** 用户运行 `wo status`
- **则** stdout 先显示当前 run 的人类进度
- **且** stdout 包含 `更新可用`
- **且** stdout 提示用户运行 `wo update`
- **且** 命令 exit code 为 0

#### 场景：无法联网检查更新

- **给定** 更新检查缓存不存在或已过期
- **且** GitHub 请求失败、超时或返回不可解析响应
- **当** 用户运行 `wo status`
- **则** stdout 仍显示当前 run 的人类进度
- **且** 命令 exit code 为 0
- **且** 不显示更新错误诊断

#### 场景：JSON status 不包含更新提示

- **给定** GitHub 上存在 `wo` 或 `oz` 新版本
- **当** 调用 `wo status --run-id <run-id> --json`
- **则** stdout 是合法 JSON 对象
- **且** JSON 字段保持现有 runner contract
- **且** stdout 不包含 `更新可用` 或 `wo update`

### 需求：wo update 安全更新 wo 和 oz

系统必须提供 `wo update` 命令，默认同时尝试更新 `wo` 和 `oz`，并在替换任何二进制前完成下载校验、新版本验证和旧版本备份。

#### 场景：两个工具都有新版本

- **给定** 本地 `wo` 和 `oz` 都低于 GitHub latest 版本
- **且** 当前平台存在匹配 Release asset
- **且** `sha256sums.txt` 校验通过
- **当** 用户运行 `wo update`
- **则** 系统备份当前 `wo` 和 `oz` 二进制
- **且** 系统安装新版二进制
- **且** stdout 显示每个工具的旧版本、新版本、备份路径和回滚命令

#### 场景：失败不影响另一个工具

- **给定** `wo` 更新成功
- **但** `oz` 更新因 checksum、版本验证、备份或下载失败而中止
- **当** 用户运行 `wo update`
- **则** 新版 `wo` 保持安装状态
- **且** 旧版 `oz` 保持不变
- **且** stdout 分别显示 `wo` 成功和 `oz` 失败原因

#### 场景：工具已经是最新

- **给定** 本地 `wo` 或 `oz` 已等于 GitHub latest 版本
- **当** 用户运行 `wo update`
- **则** 系统不下载该工具的 Release asset
- **且** stdout 明确显示该工具已是最新

#### 场景：oz 未安装

- **给定** `oz` 不在 PATH 中
- **且** `wo` 存在可用新版本
- **当** 用户运行 `wo update`
- **则** 系统仍尝试更新 `wo`
- **且** stdout 明确报告 `oz` 未安装或不可更新
- **且** 命令不会因为 `oz` 缺失而跳过 `wo` 更新

#### 场景：checksum 不匹配

- **给定** GitHub Release asset 已下载
- **且** `sha256sums.txt` 中的校验值与下载文件不匹配
- **当** 用户运行 `wo update`
- **则** 系统不得替换对应工具的当前二进制
- **且** stdout 或 stderr 明确显示校验失败

#### 场景：新二进制版本验证失败

- **给定** 下载和 checksum 校验成功
- **但** staged binary 的 `--version` 输出不等于 GitHub latest tag
- **当** 用户运行 `wo update`
- **则** 系统不得替换对应工具的当前二进制
- **且** stdout 或 stderr 明确显示版本验证失败

#### 场景：备份失败

- **给定** 下载、checksum 和 staged binary 验证都成功
- **但** 当前二进制无法备份到用户状态目录
- **当** 用户运行 `wo update`
- **则** 系统不得替换当前二进制
- **且** stdout 或 stderr 明确显示备份失败

#### 场景：Unix-like 平台更新当前 wo

- **给定** 当前平台是 Linux 或 macOS
- **当** 用户运行 `wo update`
- **则** 系统选择 `<tool>_<tag>_<goos>_<goarch>.tar.gz`
- **且** 在备份完成后用 staged binary 替换目标可执行文件

#### 场景：Windows 平台更新当前 wo

- **给定** 当前平台是 Windows
- **当** 用户运行 `wo update`
- **则** 系统选择 `<tool>_<tag>_<goos>_<goarch>.zip`
- **且** 对正在运行的 `wo.exe` 使用退出后替换流程
- **且** stdout 显示备份路径和替换状态

### 需求：审查通过后进入 QA

系统必须保持证据链完整：任一审查阶段如果确认无需修复，必须先进入同轮 QA；QA clean 且覆盖 acceptance 合同后才能进入 archive。

#### 场景：初审无需修复

- **给定** execution 已完成
- **且** review_1 的审核结果不需要修复
- **当** 系统推进下一阶段
- **则** 下一阶段必须是 `qa_1`
- **且** `wo status` 不得显示额外的修复完成标记

### 需求：JSON run abort

系统必须支持下游 runner 中止指定 run，并清理运行锁。

#### 场景：中止 run

- **当** 调用 `wo abort --run-id <run-id> --json`
- **且** 用户状态目录中的 `runs/<run-id>/state.json` 存在
- **则** 命令 exit code 为 0
- **且** stdout 是合法 JSON 对象
- **且** JSON 的 `run_id` 为 `<run-id>`
- **且** JSON 的 `status` 为 `aborted`
- **且** 对应 lock 文件不存在

### 需求：Runner-readable state file

系统必须将 sealed run 状态持久化为下游 runner 可直接读取的 JSON 文件。

#### 场景：state.json 最低字段

- **当** sealed run 已创建
- **则** 用户状态目录中的 `runs/<run-id>/state.json` 是合法 JSON 对象
- **且** 至少包含 run id、change name、status、stage、stages、paths、sessions、error
- **且** `paths` 中的路径使用项目相对 slash 路径

### 需求：JSON command error behavior

系统必须为 JSON 子命令提供可预测的失败行为。

#### 场景：JSON 命令失败

- **当** JSON 子命令因为缺少参数、未知 change、未知 run、锁冲突或坏 state 文件失败
- **则** 命令 exit code 非 0
- **且** stderr 包含人类可读错误
- **且** stdout 不输出普通文本
- **且** 如果能够确定 run id，则输出的错误 JSON 包含 `runId`、`status: "failed"`、`stage` 和 `error`

#### 场景：JSON run 后端失败

- **当** `wo run --change <change-name> --json` 已创建 sealed run
- **且** agent backend 在 execution、fix、review 或 archive 阶段失败
- **则** 命令 exit code 非 0
- **且** stdout 必须先保留启动时的 `running` DTO
- **且** stdout 必须追加一个机器可读 failed DTO
- **且** failed DTO 必须包含 `run_id`、`status: "failed"` 和非空 `error`
- **且** 用户状态目录中的 `runs/<run-id>/state.json` 必须持久化 failed 状态

### 需求：公开 CLI 命令业务测试

系统必须通过真实业务测试覆盖用户直接运行的公开命令，确保输出、错误和文件副作用稳定。

#### 场景：基础命令不依赖外部 workflow 工具

- **当** 用户运行 `wo --help` 或 `wo --version`
- **则** 命令必须成功返回
- **且** 不得要求本机存在 `oz`、`codex` 或 `opencode`
- **且** 测试必须断言 stdout 中的关键用户可见内容

#### 场景：配置命令写入预期文件

- **当** 用户在临时 git 仓库中运行 `wo config`
- **则** 仓库根目录必须生成 `wo.yaml`
- **且** 配置内容必须包含默认 workflow、validation 和 prompts 设置
- **且** 不创建 `.wo/`
- **当** 用户在非 git 目录运行 `wo config --global`
- **则** 用户主目录必须生成 `~/wo.yaml`
- **且** 不创建 `~/.wo/`
- **且** 后续 prompt 读取必须证明仓库 YAML 优先于全局 YAML，并忽略旧 `.wo/cmd`

#### 场景：旧配置命令返回迁移错误

- **当** 用户运行 `wo init` 或 `wo install`
- **则** 命令必须返回非零退出
- **且** 错误信息必须提示改用 `wo config` 或说明 prompt 已内嵌在 YAML 中

#### 场景：JSON runner 命令输出稳定结构

- **当** 用户运行 `wo contract --json`、`wo list-changes --json`、`wo status --run-id <run-id> --json` 或 `wo abort --run-id <run-id> --json`
- **则** stdout 必须是合法 JSON
- **且** 测试必须断言稳定字段名和关键状态值
- **且** 失败时必须输出机器可读错误结构或返回明确错误

### 需求：交互式 workflow 业务测试

系统必须通过真实行为测试覆盖用户在无参数运行 `wo` 时看到的关键菜单分支。

#### 场景：没有 active change 时直接进入规划

- **当** 用户无参数运行 `wo`
- **且** `oz list --json` 返回空列表
- **则** `wo` 必须直接启动 planning 阶段 agent tool
- **且** 不得显示“选择已有变更”菜单
- **当** planning 退出后仍没有 active change
- **则** `wo` 必须正常返回
- **且** 不得创建新的 batch 或 sealed run

#### 场景：存在未完成 run 时用户可以恢复或中止

- **当** 用户状态目录中的 `runs/<run-id>/state.json` 表示存在未完成 run
- **则** 交互菜单必须显示恢复、中止和开始新 run 选项
- **当** 用户选择恢复
- **则** 系统必须走 resume detached 路径
- **当** 用户选择中止
- **则** 对应 run 状态必须变为 aborted
- **且** `wo` 必须直接返回，不继续要求选择新任务

#### 场景：选择已有 oz change 后提交 sealed run

- **当** `oz list --json` 返回一个 active change
- **且** 用户选择该 change
- **则** 系统必须提交该 change 的 sealed run
- **且** 测试必须使用 fake agent runner，不能启动真实 Codex 或 OpenCode

### 需求：validation gate 失败与重试

系统必须通过 deterministic validation gate 阻止失败结果进入下一阶段，并给执行者提供可复现诊断。

#### 场景：多命令 validation 在首个失败处停止

- **当** workflow 配置了多条 validation 命令
- **且** 第一条命令失败
- **则** 后续命令不得执行
- **且** validation attempt 状态必须为 failed
- **且** state 中必须记录失败摘要

#### 场景：失败 artifact 被写入 run 目录

- **当** validation gate 失败
- **则** 系统必须写入 `validation-<stage>-<attempt>.json`
- **且** artifact 必须包含 stage、attempt、status、commands、exit_code 和 output
- **且** output 过长时必须包含截断提示

#### 场景：失败后同一阶段重试

- **当** execution 或 fix 阶段 validation 失败
- **则** 下一次运行必须重新进入同一阶段
- **且** prompt 中必须包含失败 artifact 路径和失败摘要
- **且** 未达到最大尝试次数前不得进入 review

### 需求：业务回归测试约束

系统新增测试必须围绕 workflow 使用场景，而不是只为了覆盖私有函数。

#### 场景：归档后的 shell 测试可独立运行

- **当** change 被归档
- **则** 提案 `tests/` 下的 shell 测试必须移动到根目录 `tests/`
- **且** 每个脚本必须能独立运行
- **且** 脚本必须构造临时仓库或临时 HOME
- **且** 脚本不得污染当前仓库的 `用户状态目录 runs/`
- **且** 脚本必须从脚本位置或 git 仓库根目录推导源码路径
- **且** 脚本不得依赖 `/home/zzl/projects/wo` 等维护者本机 checkout 路径

#### 场景：覆盖目标以关键路径为准

- **当** 业务测试完成
- **则** `go test ./...` 必须通过
- **且** 新增 shell 业务测试必须通过
- **且** 测试说明必须能解释每个新增测试覆盖的用户风险
- **且** 不要求达到 100% 覆盖率

### 需求：人类 status 展示并行成员摘要

系统必须在人类可读 `wo status` 中展示已到达或已产出的 parallel group 摘要，让用户不用进入 run 目录也能判断并行成员是否真正产出有效 artifact。

#### 场景：单 workflow 展示并行成员摘要

- **给定** 当前 workflow 启用了 `implementation_context` 或 `review` parallel group
- **且** run 目录存在合法 `parallel-implementation-context.json` 或 `parallel-review-<n>.json`
- **当** 用户运行 `wo status -wN`
- **则** 输出必须保留既有 `规/写/审/测` 主阶段行
- **且** 对应主阶段行下方必须展示 `并行 <group> <成功数>/<成员数> <状态>`
- **且** 必须使用配置中的用户可见成员名称和成员 artifact status

#### 场景：缺失或非法 parallel artifact 不得显示 success

- **给定** run state 中存在 parallel 子会话记录
- **且** 已到达对应 parallel group 所属阶段
- **当** 目标 `parallel-*.json` 缺失或不是合法 artifact
- **则** `wo status -wN` 必须继续成功输出
- **且** 对应并行摘要必须显示 `missing <artifact>` 或 `invalid <artifact>`
- **且** 不得仅凭 session id 显示 `success`

#### 场景：batch status 在 change 下展示并行摘要

- **给定** 当前 batch 中某个 change 已创建 run
- **且** 该 run 已到达或已产出 parallel group artifact
- **当** 用户运行 `wo status` 或 `wo status -bN`
- **则** 并行摘要必须展示在该 change 的主阶段明细下方
- **且** 不得把所有 parallel 摘要统一堆到 batch 底部

#### 场景：JSON status 不包含并行摘要

- **当** 用户运行 `wo status --run-id <run-id> --json`
- **则** JSON 字段名必须仍包含 `run_id`、`change_name`、`status`、`stage`、`stages`、`paths`、`sessions` 和 `error`
- **且** JSON 不得新增 `parallel`、`parallel_status`、`parallel_summary` 或 `members`

### 需求：默认纯 Go DAG engine

系统必须把默认 `wo run --change <change> --json` 执行路径设为内嵌纯 Go DAG engine。

#### 场景：默认 run 使用 go-dag

- **当** 用户在真实 active change 上运行 `wo run --change <change> --json`
- **则** 默认运行必须成功推进 workflow
- **且** run state 必须记录 `engine: go-dag`
- **且** 默认 workflow 配置必须启用 `parallel.enabled: true`
- **且** `wo status -wN` 必须展示 `引擎 go-dag` 和并行 group 摘要

### 需求：默认 parallel subagents 与 DAG 图

系统必须默认启用 parallel subagents，并在默认配置、DAG graph 和人类 status 中表达 planning context、implementation context、review 和 QA 的 fan-out/fan-in 语义。

#### 场景：默认配置启用 parallel

- **当** 用户运行 `wo config`
- **则** 生成的 `wo.yaml` 必须包含 `engine: go-dag`
- **且** 必须包含 `parallel.enabled: true`

#### 场景：Mermaid 图展示 fan-out/fan-in

- **当** 用户运行 `wo graph --change <change> --format mermaid`
- **则** Mermaid 输出必须包含 planning context、implementation context、review、QA 的 subagent 节点和 fan-in 节点
- **且** 图中必须包含 archive gate
- **且** 默认图不得要求任何外部 workflow scheduler

#### 场景：默认 go-dag status 保持 JSON contract 兼容

- **当** 默认 go-dag run 完成后，用户运行 `wo status --run-id <run-id> --json`
- **则** JSON 字段名必须仍包含 `run_id`、`change_name`、`status`、`stage`、`stages`、`paths`、`sessions` 和 `error`
- **且** JSON 不得新增 `parallel`、`parallel_status`、`parallel_summary` 或 `members`
