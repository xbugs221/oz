# 设计

## 决策

### 先修真实 CI 失败，不先改 workflow

最新失败 run 的 `Checkout`、`Set up Go` 和 `Build local CLIs` 均通过，失败发生在 `Run Go tests`。因此首要处理对象是 Go 测试和 prompt 合同，而不是 Actions 版本、runner 镜像或 release 打包逻辑。

```
GitHub CI
  |
  +-- Build local CLIs 通过
  |
  +-- Run Go tests 失败
        |
        +-- planning prompt 缺少“讨论规划阶段”
        +-- bundled oz skill prompt 合同缺少“讨论规划阶段”
```

### 不用弱化测试绕过失败

`wo-discuss` prompt 是用户进入规划阶段时的第一屏语义入口。只保留 `oz-plan` 技能名会让 prompt 变短，但会弱化“当前处于规划讨论阶段”的业务语义。执行阶段应优先恢复 prompt 文案和长期规格的一致性；只有在产品意图明确改变时，才允许同步更新规格、测试和 README。

### 文档覆盖维护者能执行的命令

README 需要说明 GitHub `CI` 和 `Release` 的公共门禁，至少覆盖：

- `go test ./...`
- 遍历运行根目录 `tests/*.sh`
- 本地构建 `./cmd/oz` 和 `./cmd/wo`
- GitHub 失败后如何先用本地命令复现

长期规格或 release automation 文档需要继续约束 CI/Release 使用同一套测试门禁，避免 README 和 workflow 分叉。

## 风险

- 本地 `main` 与 `origin/main` 存在历史差异，执行阶段需要明确以当前待推送代码和 GitHub 最新失败 run 双重验证，不能只在一个旧 worktree 上修补。
- `go test ./...` 在当前仓库可能耗时较长；执行阶段可以先运行本提案的定向合同测试定位，再运行完整门禁收尾。
- GitHub Actions 日志正文可能无法通过 `gh run view --log` 直接拉取；验收证据可使用 `gh run view --json` 的 run/job/step 摘要、本地复现日志和最终成功 run URL 共同证明。
