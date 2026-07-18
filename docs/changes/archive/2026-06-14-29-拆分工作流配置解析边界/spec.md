# 规格：拆分工作流配置解析边界

## 验收矩阵

| 场景 | 测试 | 证据 | 风险 |
| --- | --- | --- | --- |
| 配置解析边界拆分且用户配置行为不变 | `workflow-config-boundary-contract` | `workflow-config-boundary-log` | 既有 shell 合同耗时较 Go 单测长，执行阶段可先聚焦失败脚本 |

### 需求：工作流配置解析边界拆分

系统必须把工作流配置 schema、profile、parallel 展开和 validation 解析拆到独立文件，同时保持用户可见配置行为不变。

#### 场景：配置解析边界拆分且用户配置行为不变

- **测试文件**：`docs/changes/archive/2026-06-14-29-拆分工作流配置解析边界/tests/test_workflow_config_boundary_contract.sh`
- **真实数据来源**：仓库内置 `profiles-template`、真实 `wo config` 生成的 `wo.yaml`、临时 git 项目和既有 `tests/specs/codex-workflow-cli` 配置合同。
- **入口路径**：从仓库根目录运行 shell 合同测试，测试内部执行默认配置、legacy 拒绝、profile 发现、MADA profile 和 parallel config 业务脚本。
- **关键断言**：
  - `config_schema.go`、`config_profiles.go`、`config_parallel.go`、`config_validation.go` 必须存在。
  - `config.go` 不再直接定义 schema input、parallel 展开、profile 渲染和 validation 解析 helper。
  - 默认 tree config、legacy 字段拒绝、profile 发现和 parallel 配置合同通过。
- **剩余风险**：合同测试不覆盖未来新增配置字段；执行阶段新增字段时仍需同步补充专门场景。
