# 规格

| 验收场景 | required_tests | required_evidence |
| --- | --- | --- |
| 用户可见面不暴露内部引擎名称 | `no-internal-engine-user-surface-contract` | `internal-engine-user-surface-log` |
| 根目录历史测试层已清理 | `no-legacy-root-tests-contract` | `legacy-root-tests-log` |
| 活跃维护面不保留旧产品合同 | `current-surface-cleanup-contract` | `current-surface-cleanup-log` |

### 需求：内部引擎名称不进入用户可见面

系统必须把 `go-dag` 当作内部实现细节。非开发用户可见的文档、配置、帮助、graph、status、watch 和错误输出不得出现 `go-dag`、Dagu 或需要用户选择 engine 的叙述。

#### 场景：用户可见面不暴露内部引擎名称

- **测试文件**：`docs/changes/36-清理历史垃圾并隐藏内部引擎信息/tests/test_no_internal_engine_user_surface_contract.sh`
- **真实数据来源**：真实仓库 README、当前规格、prompt/profile 模板、`tests/specs`，以及真实编译出的 `oz` 二进制。
- **入口路径**：`go build ./cmd/oz`、`oz flow --help`、`oz flow config`、`oz flow graph --format json|mermaid`、`oz flow status`、`oz flow run --engine unknown --json`。
- **关键断言**：
  - 用户文档、当前规格、模板和当前 specs 测试不包含 `go-dag` 或 Dagu 叙述。
  - 默认生成的 `oz-flow.yaml` 不包含 engine 字段或 `go-dag`。
  - graph、status、help 和未知 engine 错误不向用户输出 `go-dag`。
  - 未知 engine 错误只说明该参数不可用或已删除，不引导用户理解内部引擎名。
- **剩余风险**：该测试不扫描 Go 源码里的内部文件名和函数名，内部实现仍可使用 `go_dag` 命名。

### 需求：根历史测试层退出活跃维护面

系统必须删除或迁移根目录 `tests/2026-*` 历史 shell 测试，避免旧 `wo` 时代脚本继续作为活跃测试入口。

#### 场景：根目录历史测试层已清理

- **测试文件**：`docs/changes/36-清理历史垃圾并隐藏内部引擎信息/tests/test_no_legacy_root_tests_contract.sh`
- **真实数据来源**：仓库真实 `tests/` 目录。
- **入口路径**：`fd '^2026-' tests`、`rg` 扫描根测试层。
- **关键断言**：
  - `tests/` 根目录下不存在 `2026-*` dated shell 测试。
  - 根测试层不再引用 `cmd/wo`、`wo.yaml`、`.wo`、`/wo/repos`、`XDG_STATE_HOME/wo` 或旧 `wo` 命令。
  - `tests/specs` 和 Go 测试继续作为当前业务测试入口。
- **剩余风险**：该测试不判断每个历史脚本业务场景是否已迁移；执行阶段需要人工归类并用 `go test ./...` 和 specs 测试兜底。

### 需求：活跃维护面不保留旧产品合同

系统必须清理当前源码、规格、测试和模板中的旧 `wo`、Dagu、legacy-agent/opencode 产品合同。旧输入拒绝测试可以保留旧字段 fixture，但不得让旧产品面继续出现在默认配置、文档、帮助或运行态路径中。

#### 场景：活跃维护面不保留旧产品合同

- **测试文件**：`docs/changes/36-清理历史垃圾并隐藏内部引擎信息/tests/test_current_surface_cleanup_contract.sh`
- **真实数据来源**：真实 `cmd/`、`internal/`、`prompts-template/`、`profiles-template/`、`README.md`、`docs/specs/`、`tests/specs/`、`.github/workflows/`、`go.mod` 和 `go.sum`。
- **入口路径**：`rg` 扫描活跃维护路径、`go test ./...`。
- **关键断言**：
  - 活跃维护面不存在独立 `cmd/wo`、`wo.yaml`、`.wo`、`/wo/repos`、旧 `wo` 命令提示或 `WO_*` 产品变量。
  - 活跃维护面不存在 Dagu 运行时或 Dagu 用户合同。
  - legacy-agent/opencode 只允许作为“旧后端被拒绝”的局部 fixture，不得出现在默认配置、文档或 profile 模板中。
  - 当前 Go 回归测试继续通过。
- **剩余风险**：扫描规则可能需要在执行阶段按“拒绝旧输入 fixture”做少量白名单，但白名单必须窄到具体测试文件和具体 heredoc。
