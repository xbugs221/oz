# 执行任务

本文记录升级后动态质量循环验证的执行状态，供 execution artifact gate 确定性检查。

- [x] 1.1 在默认修复提示中明确动态 `audit_N` 与 `targeted_repair_N` 阶段映射。
- [x] 1.2 安装当前本地源码构建的 `oz`，运行封存 acceptance 声明的安装版合同测试。
- [x] 1.3 运行 `go test ./internal/app` 及动态质量循环相关长期规格测试。
