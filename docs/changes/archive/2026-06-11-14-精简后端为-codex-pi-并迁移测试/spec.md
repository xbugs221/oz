# 规格：精简后端为 Codex/Pi 并迁移测试

## 验收矩阵

| 需求 | 场景 | required_tests | required_evidence |
| --- | --- | --- | --- |
| 需求：后端集合只保留 Codex/Pi | 场景：仓库无旧后端残留 | `contract-backend-removal` | `backend-removal-log` |
| 需求：后端集合只保留 Codex/Pi | 场景：配置只接受 Codex/Pi | `contract-cli-preflight` | `cli-preflight-log` |
| 需求：启动前检查 CLI 工具 | 场景：任一必需 CLI 缺失时失败且不创建运行态 | `contract-cli-preflight` | `cli-preflight-log`, `state-clean-snapshot` |
| 需求：源码和测试分离 | 场景：`internal/` 不再保存长期测试 | `contract-root-test-layout` | `root-test-layout-log` |
| 需求：文档和发布门禁同步 | 场景：主规格和发布测试只承诺 Codex/Pi 与根目录测试 | `contract-docs-release-gate` | `docs-release-gate-log` |

### 需求：后端集合只保留 Codex/Pi

系统必须只支持 `codex` 和 `pi` 两个 agent CLI。旧后端的源码、测试、配置示例、状态兼容、帮助文案、规格文档和长期测试都必须删除。

#### 场景：仓库无旧后端残留

- **给定** 当前仓库完成实现
- **当** 扫描除本提案说明以外的源码、文档、测试和归档材料
- **则** 不得再出现旧后端名称、旧 runner 类型、旧 session key 或旧可执行文件夹具
- **对应测试**：`docs/changes/archive/2026-06-11-14-精简后端为-codex-pi-并迁移测试/tests/test_backend_removal_contract.sh`
- **真实数据来源**：当前仓库真实文件树
- **入口路径**：仓库根目录
- **关键断言**：仓库搜索结果为空，旧后端实现文件不存在
- **剩余风险**：二进制发布包里的历史内容不由源码扫描覆盖

#### 场景：配置只接受 Codex/Pi

- **给定** 用户配置 workflow stage tool
- **当** 配置值不是 `codex` 或 `pi`
- **则** 配置校验返回未知 agent tool
- **且** sealed run 不启动
- **对应测试**：`docs/changes/archive/2026-06-11-14-精简后端为-codex-pi-并迁移测试/tests/test_cli_preflight_contract.sh`
- **真实数据来源**：临时真实 git 仓库、真实编译后的 `wo` CLI、真实 `wo.yaml`
- **入口路径**：`wo run --change demo --json`
- **关键断言**：旧后端配置被拒绝，错误说明为未知工具
- **剩余风险**：测试只覆盖 CLI 入口，不单独覆盖每个配置解析 helper

### 需求：启动前检查 CLI 工具

系统必须在 sealed run 启动前检查宿主机同时存在 `codex` 和 `pi`。任一缺失时，必须提示用户先安装缺失 CLI，并且不得创建 run state。

#### 场景：任一必需 CLI 缺失时失败且不创建运行态

- **给定** 临时 PATH 中只有 fake `codex`，没有 `pi`
- **当** 用户运行 `wo run --change demo --json`
- **则** 命令在 agent 执行前失败
- **且** 输出明确提到缺失的 `pi` 和安装指引
- **给定** 临时 PATH 中只有 fake `pi`，没有 `codex`
- **当** 用户运行 `wo run --change demo --json`
- **则** 命令在 agent 执行前失败
- **且** 输出明确提到缺失的 `codex` 和安装指引
- **且** 用户状态目录下不出现 `runs/` 运行态
- **对应测试**：`docs/changes/archive/2026-06-11-14-精简后端为-codex-pi-并迁移测试/tests/test_cli_preflight_contract.sh`
- **真实数据来源**：临时真实 git 仓库、真实编译后的 `wo` CLI、受控 PATH
- **入口路径**：`wo run --change demo --json`
- **关键断言**：缺失 `pi` 或 `codex` 时失败、包含安装提示、不创建状态
- **剩余风险**：安装指引不校验具体 URL，只要求人能理解下一步

### 需求：源码和测试分离

长期测试必须位于仓库根目录 `docs/changes/archive/2026-06-11-14-精简后端为-codex-pi-并迁移测试/tests/` 下，`internal/` 只保留生产源码。执行阶段可以重写测试形态，但不得降低业务覆盖。

#### 场景：`internal/` 不再保存长期测试

- **给定** 当前仓库完成测试迁移
- **当** 扫描 `internal/` 目录
- **则** 不存在 `*_test.go`
- **并且** 根目录 `docs/changes/archive/2026-06-11-14-精简后端为-codex-pi-并迁移测试/tests/` 下存在可运行的 Go 或 shell 业务测试入口
- **对应测试**：`docs/changes/archive/2026-06-11-14-精简后端为-codex-pi-并迁移测试/tests/test_root_test_layout_contract.sh`
- **真实数据来源**：当前仓库真实文件树
- **入口路径**：仓库根目录
- **关键断言**：`internal/**/_test.go` 数量为 0，根目录测试入口非空
- **剩余风险**：测试迁移后的覆盖强度需在执行阶段通过长期回归测试补充确认

### 需求：文档和发布门禁同步

主规格、发布合同和根目录长期测试必须与新的后端集合和测试布局一致。

#### 场景：主规格和发布测试只承诺 Codex/Pi 与根目录测试

- **给定** 当前仓库完成实现
- **当** 检查主规格和发布门禁测试
- **则** 文档只声明 `codex` 和 `pi`
- **且** 文档说明启动前检查两个 CLI
- **且** 发布门禁使用根目录 `docs/changes/archive/2026-06-11-14-精简后端为-codex-pi-并迁移测试/tests/` 作为长期测试入口
- **对应测试**：`docs/changes/archive/2026-06-11-14-精简后端为-codex-pi-并迁移测试/tests/test_docs_release_gate_contract.sh`
- **真实数据来源**：`docs/specs/codex-workflow-cli/spec.md` 和根目录发布测试
- **入口路径**：仓库根目录
- **关键断言**：规格和发布门禁不再承诺旧后端或 `internal/` 测试布局
- **剩余风险**：外部 README 如后续新增，需要单独纳入文档扫描
