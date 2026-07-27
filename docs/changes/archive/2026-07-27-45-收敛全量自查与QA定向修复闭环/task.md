# 任务

## 1. 先运行创建阶段合同

- [x] 运行 `bash docs/changes/archive/2026-07-27-45-收敛全量自查与QA定向修复闭环/tests/test_quality_delivery_loop.sh`
- [x] 确认初始失败指向有限 repair 状态机、QA 后全量扩审或缺少环境阻塞语义，而不是测试语法和路径

## 2. 建立动态质量循环

- [x] 为新 run 引入全量 `pre_qa_audit` 与 `qa_targeted_repair` 模式
- [x] 动态追加 audit、repair、QA stage，不按 `max_repair_iterations` 预生成终止边界
- [x] 保持旧 sealed `repair-v1` run 按快照恢复
- [x] 更新 graph、status、contract JSON 和配置迁移诊断

## 3. 落实 QA 定向修复与自测门禁

- [x] 将最新 QA findings、失败 acceptance IDs 和 QA artifact 路径写入 targeted repair prompt
- [x] 限定 targeted repair 处理 findings 与直接相关回归，不重新启动全量 audit
- [x] 强制复跑失败测试、全部 required tests 和 validation commands
- [x] 将测试结果与当前 diff 哈希绑定，未通过或过期时不得移交 QA

## 4. 分离环境阻塞和停滞阻塞

- [x] 增加 `blocked_environment`，只记录缺失路径/变量名而不记录密钥值
- [x] 增加基于 finding、diff、tests、evidence 哈希的 `blocked_stalled`
- [x] `resume/restart` 在条件变化后从原阶段恢复
- [x] 删除新运行基于轮次触发 `blocked_review_limit` 的路径

## 5. 补齐真实测试和交付验证

- [x] 新增真实 Engine 集成测试：全量自查确认后进入 QA
- [x] 新增真实 Engine 集成测试：QA findings 定向修复、自测后再次 QA
- [x] 新增 12 次 needs_fix 后最终 clean 的无上限回归
- [x] 新增环境阻塞与停滞阻塞恢复回归
- [x] 运行合同脚本、`go test ./internal/app`、`go test ./...`
- [x] 更新长期规格与版本说明

## 历史测试更新原因

`test_compact_chinese_graph_and_iteration_limit.sh`、
`test_remove_fixed_subagents_contract.sh`、
`test_stage_artifact_gate_retry_contract.sh`、
`test_stage_prompt_contract_completeness.sh`、
`test_role_prompt_first_turn_contract.sh` 与
`test_execution_prompt_hard_contract_focus.sh` 原先把默认有限 repair 节点或
无封存 acceptance 的内存状态作为长期合同，
与本提案“不按固定轮次终止”的新意图冲突；现已改为验证动态
`audit_N / qa_N / targeted_repair_N` 模板及封存合同 fail-closed 行为。
旧 sealed `repair-v1` 提示词兼容继续由显式快照 fixture 和 Go 回归覆盖。

## 6. 全量自查补充修复

- [x] 分离原始门禁完整性哈希与稳定停滞进展哈希，过滤耗时、时间戳和尝试序号噪声
- [x] QA/archive 阻塞恢复在证据变化或人工重启后重新经过可信门禁
- [x] 定向恢复前校验来源 QA acceptance 并刷新 finding 指纹
- [x] 补充挥发日志、证据恢复和真实 pre-QA 源码修复回归

## 7. 续轮兼容修复

- [x] 兼容同代旧 acceptance checkpoint 缺少派生进展哈希
- [x] 保持新 checkpoint 进展哈希与原始日志的严格防篡改校验
- [x] 将来源 QA 完整内容与已通过的只读输入门禁绑定
- [x] evidence 语义进展保留实质内容变化并过滤已知运行噪声
- [x] 覆盖带时区偏移的 RFC3339 时间戳噪声

## 8. 来源 QA 与停滞恢复信任闭环

- [x] 定向修复提示词重放来源 QA 只读门禁与持久检查点
- [x] 保留生产 QA 只读门禁类型，并为检查点哈希建立阶段绑定
- [x] clean QA 完整内容在 archive 前后均 fail-closed 校验
- [x] stalled 恢复只以 evidence 语义变化作为证据进展
- [x] evidence 规范化按 kind 收窄，并对大型 artifact 使用流式哈希

## 9. 第四轮全量自查修复

- [x] QA 门禁绑定 validation 与 acceptance 持久 artifact 的完整字节
- [x] 同代旧定向修复状态在运行入口 fail-closed 转入新 audit
- [x] fresh audit 重置旧 archive invariant，归档成功记录 passed 门禁
- [x] state snapshot 只规范化引擎记录并拒绝静默忽略尾随 JSON
- [x] 大型 runtime log 流式过滤耗时噪声且保留实质变化

## 10. 第五轮全量自查修复

- [x] QA 执行前冻结完整检查点，并拒绝执行期间发生的检查点漂移
- [x] 旧定向修复制品在完成快捷路径前经过来源 QA 信任迁移
- [x] acceptance 原始与进展哈希使用同一文件观察并持久化绑定
- [x] evidence 缺失、不可读或超大状态快照不得作为可信恢复进展
- [x] 补齐 DAG、worker、attempt、派生路径与超长运行日志噪声边界

## 11. 第六轮 QA 定向修复

- [x] 后台 resume、restart 与 batch restart 启动失败后恢复原阻塞状态
- [x] Git 快照逐文件记录既存未跟踪目录，拒绝夹带未声明文件
- [x] 环境 marker 脱敏丢弃不可信前缀并覆盖四类持久产物
- [x] 动态运行图按持久化阶段时间展示真实执行顺序
- [x] 状态视图将 QA 与归档环境阻塞映射到真实阶段

## 12. 后台 worker 交接边界修复

- [x] 区分进程启动前失败与启动后句柄交接失败
- [x] 已启动 worker 的交接错误不回滚 prepared run/batch 状态
- [x] 覆盖 resume、restart、batch 与真实进程释放失败回归

## 13. Archive 恢复位置回滚

- [x] detached 回滚快照记录原 active/archived 提案位置
- [x] worker 启动前失败时恢复提案目录及 run/batch 状态
- [x] 覆盖 resume、restart、batch 的 archive 阻塞恢复

## 14. 第八轮 QA 恢复事务加固

- [x] prepare 任一步失败均补偿 run、batch 与 archive 提案位置
- [x] detached 回滚取得 run lease 并以持久 state 代次执行 CAS
- [x] 环境阻塞持久化 dirty 路径内容检查点并拒绝内容绕过
- [x] 覆盖 prepare 部分提交、并发进展及既存 dirty 内容变化
