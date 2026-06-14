# 提案：拆分工作流 Engine 运行边界

## 背景

`state.go` 已经不只是状态定义文件。它还承担运行锁、恢复策略、阶段执行、agent progress 解析、session merge、manual intervention 检查等职责。任何小改动都容易牵连工作流核心路径。

## 目标

- 把持久化状态模型和 Engine 编排代码拆开。
- 把 resume/lock 恢复逻辑从普通 run loop 中分离。
- 把 stage 执行逻辑从 run loop 中分离。
- 把 progress writer 和 session 持久化从状态模型文件中分离。
- 保持 `go test ./internal/app` 中 workflow、stage、restart、状态展示相关回归继续通过。

## 非目标

- 不改变 `State` JSON 字段。
- 不重写状态机决策。
- 不调整默认 workflow engine。
