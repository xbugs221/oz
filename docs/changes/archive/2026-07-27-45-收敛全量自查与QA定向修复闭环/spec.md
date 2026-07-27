# 规格：高效低缺陷交付闭环

### 需求：QA 前完成当前提案的全量自查

#### 场景：全量自查修复后再次确认再进入 QA

- **给定** execution 已完成且当前提案 acceptance 可运行
- **当** 全量自查发现问题并完成代码修复
- **则** 工作流必须复跑 required tests 并追加下一次全量自查
- **并且** 只有一次全量自查未发现新问题且自测全部通过，才创建独立 QA 阶段
- **测试**：`docs/changes/archive/2026-07-27-45-收敛全量自查与QA定向修复闭环/tests/test_quality_delivery_loop.sh`
- **真实数据来源**：临时 Git 仓库中的真实 change、acceptance、run state、repair artifact 与 `internal/app` 引擎
- **入口路径**：`oz flow run/resume`
- **关键断言**：execution 后先 audit；修复后仍 audit；clean 且自测通过后才 QA
- **剩余风险**：模型是否发现全部问题仍由独立 QA 和回归测试兜底

### 需求：QA 打回后只进行有证据的定向修复

#### 场景：定向修复复跑失败测试和完整验收后移交 QA

- **给定** 独立 QA 返回 blocking findings 和失败 acceptance IDs
- **当** repairer 处理 QA 打回
- **则** prompt 必须包含最新 QA artifact，并将工作范围限定为 findings 与直接相关回归
- **并且** repairer 必须复跑失败测试、全部 required tests 和 validation commands
- **并且** 只有这些测试均通过且结果绑定当前 diff 哈希，才能创建下一轮独立 QA
- **测试**：`docs/changes/archive/2026-07-27-45-收敛全量自查与QA定向修复闭环/tests/test_quality_delivery_loop.sh`
- **真实数据来源**：真实 QA JSON、acceptance JSON、命令运行记录和引擎持久状态
- **入口路径**：`oz flow run/resume`
- **关键断言**：QA 后不回到全量 audit；未自测通过不移交；通过后进入新的隔离 QA 会话
- **剩余风险**：findings 的语义质量取决于 QA，但 acceptance 全量复跑防止明显回归

### 需求：质量循环不因固定轮次终止

#### 场景：超过历史十轮限制后仍能完成交付

- **给定** 一个运行连续经历至少 12 次 QA needs_fix 和定向修复
- **当** 第 13 次 QA clean 且所有验收通过
- **则** 工作流必须进入 archive 并完成
- **并且** 不得产生 `blocked_review_limit` 或其它基于轮次数量的失败
- **测试**：`docs/changes/archive/2026-07-27-45-收敛全量自查与QA定向修复闭环/tests/test_quality_delivery_loop.sh`
- **真实数据来源**：引擎动态追加的真实 stage records 和 artifacts
- **入口路径**：`oz flow run/resume/status/graph`
- **关键断言**：12 次修复后仍可继续；最终 QA clean 可归档；状态展示不依赖预生成上限
- **剩余风险**：长运行会增加状态文件体积，需要保持增量持久化

### 需求：环境问题与无进展从代码质量循环分离

#### 场景：缺失环境补齐后从原阶段恢复

- **给定** evidence producer 需要的账户配置或外部服务不可用
- **当** preflight 能明确识别缺失路径或变量名
- **则** 工作流进入 `blocked_environment` 并记录无密钥值的诊断
- **并且** 条件补齐后 `resume/restart` 从原 audit 或 targeted repair 阶段继续
- **并且** 环境阻塞不得增加失败修复计数，也不得改写为 `needs_more`
- **测试**：`docs/changes/archive/2026-07-27-45-收敛全量自查与QA定向修复闭环/tests/test_quality_delivery_loop.sh`
- **真实数据来源**：临时 HOME、真实配置探测、run state 与恢复命令
- **入口路径**：`oz flow run/resume/restart/status`
- **关键断言**：缺环境时可诊断暂停；补齐后原阶段恢复；日志不包含密钥值
- **剩余风险**：无法自动探测的第三方权限仍需 agent 写出明确人工检查项

#### 场景：相同失败且无任何变化时暂停等待新信息

- **给定** 相邻两次失败具有相同 finding 指纹
- **当** 源码、测试、验证和 evidence 哈希均无变化
- **则** 工作流进入 `blocked_stalled`
- **并且** 状态说明恢复所需的新代码、证据、配置或人工指令
- **并且** 不得通过固定轮次上限替代进展判断
- **测试**：`docs/changes/archive/2026-07-27-45-收敛全量自查与QA定向修复闭环/tests/test_quality_delivery_loop.sh`
- **真实数据来源**：连续 stage artifacts、Git diff 哈希和验证记录
- **入口路径**：`oz flow run/resume/status`
- **关键断言**：有变化时持续修复；无变化时暂停；提供新输入后可以恢复
- **剩余风险**：指纹归一化需避免把同一问题的措辞变化误判为进展
