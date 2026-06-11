# 提案：允许运行中追加新需求但保留 subagent 写保护

## 为什么

运行中的 `wo` workflow 需要防止 agent backend 和只读 subagent 产生不可控修改。但用户在同一仓库里新增另一个 `docs/changes/<新需求>/` 是正常协作行为，不应导致当前 run 中止。现在的全局 git diff 检测把这两类行为混在一起，降低了工作流可用性。

## 做什么

- 把“人工干预中止”改成路径感知的工作区保护。
- 当前 run 允许的外部变化仅限非当前 change 目录，例如 `docs/changes/16-新需求/`。
- 当前 change、源码目录、测试、配置、主规格、profile/prompt 模板等路径发生变化时仍阻断。
- 只读 subagent 执行前后发现仓库业务文件变化时必须失败，并报告变更路径。
- 主规格和长期测试同步说明：保护目标是 subagent 错误写入和当前 run 污染，不是禁止用户新增需求。

## 不做什么

- 不自动判断 git 作者或进程 PID。
- 不自动 stash、revert 或提交用户新增的需求。
- 不让新增需求影响当前 run 的 acceptance、prompt 或 review 范围。
- 不放宽当前 change 的运行期修改限制。

## 可验证结果

- 运行中新增 `docs/changes/<非当前提案>/brief.md` 后，当前 run 可继续执行。
- 运行中修改当前 change 或源码文件时，当前 run 仍中止或失败。
- 错误文案能提示被阻断路径属于当前 run 相关路径或源码变化。
