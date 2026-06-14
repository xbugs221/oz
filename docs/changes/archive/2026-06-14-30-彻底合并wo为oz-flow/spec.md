# 规格：彻底合并 wo 为 oz flow

## 验收矩阵

| 场景 | 测试 | 证据 | 风险 |
| --- | --- | --- | --- |
| 仓库只构建一个 oz 二进制且 flow 命令组可用 | `single-oz-flow-binary-contract` | `single-oz-flow-binary-log` | 不启动真实 agent 后端，只验证 CLI 分发、帮助、状态类只读入口和发布面 |
| 活跃产品面不再残留 wo 命名或兼容合同 | `no-wo-legacy-surface-contract` | `no-wo-legacy-surface-log` | 历史归档提案不扫描，保留其历史文字 |

### 需求：唯一 oz CLI 承载工作流命令

系统必须删除独立 `wo` 命令入口，CI 和 Release 只构建 `oz`，并通过 `oz flow` 命令组暴露原工作流能力。

#### 场景：仓库只构建一个 oz 二进制且 flow 命令组可用

- **测试文件**：`docs/changes/30-彻底合并wo为oz-flow/tests/test_single_oz_flow_binary_contract.sh`
- **真实数据来源**：仓库真实 Go CLI 入口、GitHub Actions 配置、README、当前 git 临时项目和 `oz flow` 实际命令输出。
- **入口路径**：从仓库根目录运行 shell 合同测试，测试内部执行 `go build -o "$tmp/oz" ./cmd/oz` 和多个 `oz flow` 只读命令。
- **关键断言**：
  - `cmd/oz/main.go` 存在，`cmd/wo` 不存在。
  - CI/Release workflow 不再引用 `./cmd/wo`、`cmd/wo` 或独立 `wo` 二进制产物。
  - `oz --help` 展示 `flow` 命令组。
  - `oz flow --help`、`oz flow status` 和 `oz flow watch` 能通过真实二进制执行。
  - `go test ./...` 通过，证明合并后 Go 代码仍自洽。
- **剩余风险**：测试不启动真实 agent 写代码流程；执行阶段可按影响面补跑具体 workflow shell specs。

### 需求：删除 wo 历史兼容产品面

系统不得保留 `wo` 兼容命令、旧配置名、旧状态目录名、旧发布合同或活跃文档中的旧产品命名。用户可见命令提示必须指向 `oz flow`。

#### 场景：活跃产品面不再残留 wo 命名或兼容合同

- **测试文件**：`docs/changes/30-彻底合并wo为oz-flow/tests/test_no_wo_legacy_surface_contract.sh`
- **真实数据来源**：仓库活跃源码目录、模板目录、README、GitHub Actions、长期 specs/tests 和 Go module 元数据。
- **入口路径**：从仓库根目录运行 shell 合同测试，测试内部使用 `rg` 扫描当前产品路径。
- **关键断言**：
  - 活跃产品路径中不存在 `cmd/wo`、`./cmd/wo`、`go install .../cmd/wo`、`wo.yaml`、`XDG_STATE_HOME/wo`、`state_home/wo` 等旧合同。
  - 活跃用户文档和规格中不再出现 `wo status`、`wo watch`、`wo run`、`wo clean`、`wo config` 等旧命令提示。
  - 模板文件名不再使用 `wo-*.md`。
  - 不保留 `github.com/xbugs221/wo` 依赖或导入。
- **剩余风险**：不扫描 `docs/changes/archive/`，因为归档提案是历史记录，不代表当前产品面。
