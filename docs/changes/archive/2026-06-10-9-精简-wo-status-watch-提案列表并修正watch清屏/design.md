# 设计

## 输出结构

本次把 human 输出的顶层语义调整为“提案列表”，而不是“运行对象编号”：

```text
- <change-name>
  <阶段名> <session-id|- > <marker> <minutes|->
    <子代理名> <session-id|- > <marker> <minutes|->
```

单 workflow 也使用同样结构：

```text
- 198-严格验证Kestra真实UI嵌入并补齐运维复现文档
  规划阶段 - - -
  执行阶段 writer-session → -
```

batch 只是多个 workflow 提案列表顺序拼接：

```text
- 198-严格验证Kestra真实UI嵌入并补齐运维复现文档
  规划阶段 - - -
- 199-另一个提案
```

`b1`、`w1` 仍可作为命令参数定位对象，但不在默认 human 输出中显示。

## spinner 规则

`statusViewRow.Marker` 继续表达稳定阶段状态：

- `-`：未开始或没有可见状态
- `→`：正在运行
- `✓`、`✓✓`：完成轮次
- `x`：失败或阻塞

`wo status` 直接渲染这些 marker。

`wo watch` 渲染时只把当前帧的 running marker `→` 替换为 spinner 帧：

```text
执行阶段 writer-session | -
执行阶段 writer-session / -
执行阶段 writer-session - -
执行阶段 writer-session \ -
```

顶部不再显示 spinner，避免旧帧 header 成为残留源。

## 渲染职责

建议拆出两层 helper：

```text
statusView
  rows[]
        |
        v
workflowProposalLines(changeName, view, markerMode)
        |
        v
batchProposalLines(batch, markerMode)
```

`markerMode` 只决定 running marker 是否替换为 spinner，不决定输出层级。

这样 `wo status`、`wo watch`、batch 内 workflow 展示都复用同一套列表渲染，避免再次出现 `status` 和 `watch` 漂移。

## TTY 清屏

当前实现用上一帧逻辑行数做：

```text
\x1b[<prev-lines>A\x1b[J
```

风险在于一个逻辑行可能在终端中换成多行，尤其是中文提案名。可选实现：

- 使用整屏刷新：每帧写 `\x1b[H\x1b[2J` 后重新绘制。
- 或按终端宽度和 Unicode 显示宽度计算实际屏幕行数，再回退清理。

本次优先推荐整屏刷新，逻辑简单，避免引入宽字符计算风险。非 TTY 输出仍保留多帧追加，便于脚本捕获 watch 动画。

## 风险

- 旧测试中明确要求 `watch` spinner 位于 header，需要按新意图更新。
- 部分用户可能习惯在输出中看到 `b1/w1`，但命令参数仍保留这些别名，必要时可通过后续变更增加显式详情命令。
- 伪 TTY 合同依赖系统 `script` 命令；这是测试层依赖，不进入运行时代码依赖。
