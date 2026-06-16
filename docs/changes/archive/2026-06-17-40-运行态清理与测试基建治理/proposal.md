# 提案：运行态清理与测试基建治理

## 问题

`oz flow clean` 现在边扫描边删除 failed、blocked、aborted、corrupt run/batch 和可选 agent session 记录。它已经有保护 active lock 的逻辑，但用户和测试无法在删除前看到完整计划。对运行态清理这种高风险文件操作，先生成 plan 再 apply 更容易审查和回归。

另一方面，核心 workflow 测试中的 fake runner、临时 repo、acceptance writer、git helper 分散在长测试文件里。后续改 Engine、DAG 或 gate 时，会反复复制 fixture 逻辑。

## 目标

- 拆出 `BuildCleanPlan` 和 `ApplyCleanPlan`。
- 新增 `oz flow clean --dry-run --json`，输出将删除、跳过、保护的 run/batch/session 及原因。
- 保持 `oz flow clean` 默认行为不变，只是内部从 plan apply。
- 提取 workflow 测试夹具，复用 fake agent、acceptance writer、git helper。
- 顺手收敛重复的 version/git helper，作为低风险清理项。

## 非目标

- 不改变 clean 默认删除范围。
- 不对运行中任务做强制删除。
- 不清理当前项目源码或 test-results。
- 不把所有 specs shell 测试改写成 Go。

## 验收

创建阶段契约测试必须通过：

```bash
bash docs/changes/40-运行态清理与测试基建治理/tests/clean_plan_and_fixture_contract_test.sh
```

执行阶段还必须运行：

```bash
go test ./internal/app
go test ./...
```
