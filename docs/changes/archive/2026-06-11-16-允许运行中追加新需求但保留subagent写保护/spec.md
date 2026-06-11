# 规格：允许运行中追加新需求但保留 subagent 写保护

## 验收矩阵

| 场景 | required_tests | required_evidence |
| --- | --- | --- |
| 需求：运行中允许新增非当前需求 / 场景：阶段开始前发现另一个 active change | `contract-running-demand-insertion` | `running-demand-log` |
| 需求：当前 run 相关修改仍被阻断 / 场景：源码或当前 change 被修改 | `contract-running-demand-insertion` | `running-demand-log` |
| 需求：主规格表达保护边界 / 场景：文档不再承诺禁止一切工作区变化 | `contract-manual-intervention-docs` | `manual-intervention-docs-log` |

### 需求：运行中允许新增非当前需求

系统必须允许用户在 sealed run 执行期间新增或编辑另一个 `docs/changes/<非当前 change>/`。这些变化不得中止当前 run，也不得改变当前 run 的 acceptance、prompt 或 review 范围。

#### 场景：阶段开始前发现另一个 active change

- **给定** 当前 run 的 `change_name` 是 `10-当前需求`
- **且** run 已记录初始 `BaselineHead` 和 `BaselineDiff`
- **当** 用户新增 `docs/changes/11-运行中新需求/brief.md`
- **且** 阶段边界执行 git snapshot 检查
- **则** 当前 run 不得写为 `aborted_manual_intervention`
- **且** 检查必须更新 baseline，后续阶段不会反复报告同一新增需求
- **对应测试**：`docs/changes/16-允许运行中追加新需求但保留subagent写保护/tests/test_running_demand_insertion_contract.sh`
- **真实数据来源**：临时 git 仓库、真实 `State`、真实 `detectManualIntervention`
- **入口路径**：`go test ./tests/app`
- **关键断言**：新增非当前 change 后不报错、不 abort、baseline 包含新 diff
- **剩余风险**：测试不模拟用户编辑器进程，只验证最终 git snapshot 行为

### 需求：当前 run 相关修改仍被阻断

系统必须继续阻断源码、测试、配置、当前 change 和主规格等会污染当前 run 语义的变化。只读 subagent 造成这些变化时，也必须被视为违规。

#### 场景：源码或当前 change 被修改

- **给定** 当前 run 尚未进入可写主阶段
- **当** 工作区出现 `internal/app/rogue_write.go` 或 `docs/changes/10-当前需求/spec.md` 修改
- **则** 检查必须失败
- **且** run 状态必须写为 `aborted_manual_intervention`
- **且** 错误信息必须指出阻断原因是当前 run 相关路径或源码变化
- **对应测试**：`docs/changes/16-允许运行中追加新需求但保留subagent写保护/tests/test_running_demand_insertion_contract.sh`
- **真实数据来源**：临时 git 仓库、真实 `State`、真实 `detectManualIntervention`
- **入口路径**：`go test ./tests/app`
- **关键断言**：源码和当前 change 修改仍失败；错误不是静默继续
- **剩余风险**：路径分类不能证明修改者身份

### 需求：主规格表达保护边界

主规格必须把人工干预保护描述为“路径感知保护”，不能继续表达为禁止所有无法归因的工作区变化。

#### 场景：文档不再承诺禁止一切工作区变化

- **当** 执行文档门禁测试
- **则** `docs/specs/codex-workflow-cli/spec.md` 必须说明非当前 `docs/changes/<change>/` 可在运行中新增
- **且** 必须说明当前 change、源码和配置变化仍会中止
- **对应测试**：`docs/changes/16-允许运行中追加新需求但保留subagent写保护/tests/test_manual_intervention_docs_contract.sh`
- **真实数据来源**：主规格文档
- **入口路径**：`bash docs/changes/16-允许运行中追加新需求但保留subagent写保护/tests/test_manual_intervention_docs_contract.sh`
- **关键断言**：规格包含允许新增非当前需求和保留 subagent 写保护的描述
- **剩余风险**：文档测试不证明实现，只防止规格回退
