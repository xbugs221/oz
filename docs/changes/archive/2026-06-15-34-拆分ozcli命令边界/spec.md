# 规格：拆分 ozcli 命令边界

## 验收矩阵

| 需求 | 场景 | required_tests | required_evidence | 关键断言 |
| --- | --- | --- | --- | --- |
| ozcli 命令边界拆分 | 入口、install、change、validate、archive 命令被分离 | ozcli-boundary | ozcli-boundary-log | 目标文件存在，关键函数落在对应文件，ozcli Go 回归仍通过 |

### 需求：ozcli 命令边界拆分

standalone `oz` CLI 必须按命令职责拆分文件，避免 `ozcli.go` 继续混合安装、提案查询、校验、归档和版本定位逻辑。

#### 场景：入口、install、change、validate、archive 命令被分离

- 测试文件：`docs/changes/34-拆分ozcli命令边界/tests/ozcli_boundary_test.sh`
- 真实数据来源：仓库当前 `internal/ozcli` 生产代码和现有 `internal/ozcli` Go 回归测试。
- 入口路径：执行 shell 契约测试，内部检查目标 Go 文件和运行 `go test ./internal/ozcli`。
- 关键断言：`Main/run`、`installCmd`、`list/create/status`、`validateChange`、`archiveCmd` 必须分别位于目标文件；原 `ozcli.go` 不得继续是 700 行以上混合职责文件；现有 ozcli 回归必须通过。
- 剩余风险：该测试不额外跑 release 打包流程，执行阶段如触及构建入口需补跑 release 相关合同。
