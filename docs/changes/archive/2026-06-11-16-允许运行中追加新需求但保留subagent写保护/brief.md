# 简报：允许运行中追加新需求但保留 subagent 写保护

## 用户问题

`wo` 在 sealed workflow 执行期间会检测 git 工作区变化。这个保护的本意是发现只读 subagent 擅自修改源码或当前变更，但当前行为过宽：用户在工作流运行期间新增另一个需求提案，也可能被当成人工干预并中止当前 run，导致无法中途记录新需求。

## 交付目标

- 允许用户在某个 run 执行期间新增或修改“非当前 change”的 `docs/changes/<其他提案>/` 内容。
- 当前 run 不因这些非当前需求提案变化而中止，并把新的 git snapshot 记录为后续基线。
- 仍然阻止只读 subagent 或阶段边界出现的源码、当前 change、配置、测试和运行外产物的未授权修改。
- 用户可见错误信息必须说明阻断的是“当前 run 相关路径或源码变化”，不是笼统禁止所有 git 状态变化。

## 非目标

- 不允许用户直接修改当前正在执行的 change 并让当前 run 静默继续。
- 不允许 subagent 修改任何仓库文件。
- 不自动把新增需求插入当前已启动 batch；追加到 batch 仍走既有追加机制。
- 不引入文件锁、后台守护或复杂作者归因系统。

## 验收入口

执行阶段必须先运行本提案 `docs/changes/archive/2026-06-11-16-允许运行中追加新需求但保留subagent写保护/tests/` 下的契约测试。当前实现应在“运行中新增非当前 change”场景失败，修复后必须通过，并补充根目录长期回归测试。

## 执行阶段默认上下文

核心代码在 `internal/app/state.go` 的 `detectManualIntervention` 和 `gitSnapshot`。现状只比较 `BaselineHead` / `BaselineDiff`，在 `len(state.Stages) == 0` 时直接中止。目标不是删除保护，而是把保护收窄到当前 run 相关路径和只读 subagent 违规写入。
