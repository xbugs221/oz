# 设计：硬删除旧后端并收敛测试布局

## 技术决策

### 后端集合固定化

`AgentRegistry` 只注册 `CodexTool` 和 `PiTool`。配置校验函数只接受这两个名字。执行阶段不再需要第三后端参数构造、JSONL session 解析、stderr 包装或 runner 集成测试。

```text
workflow config
      |
      v
valid tool? codex/pi only
      |
      v
preflight: codex exists && pi exists
      |
      v
sealed run
```

### 启动前工具检查

启动 sealed run 前执行统一 preflight：

```text
required host tools:
  - codex
  - pi

missing tool:
  fail early
  print install guidance
  do not create runs/<run-id>
```

这里不按当前 workflow 是否实际使用某个工具裁剪检查范围。用户要求启动前检查两个 CLI，且目标是降低运行中才失败的复杂度。

### 测试迁移

迁移后的结构应保持简单：

```text
internal/
  app/
    *.go              # production source only

docs/changes/archive/2026-06-11-14-精简后端为-codex-pi-并迁移测试/tests/
  app/                # Go 行为测试或 CLI 驱动测试
  specs/              # 长期规格测试
```

原来依赖同包未导出函数的测试，优先改成 CLI、导出 API 或根目录 shell 规格测试。确实需要保留的底层断言，应通过更稳定的业务入口覆盖，避免为了测试迁移而扩大导出面。

## 风险

- 一次性删除旧后端会触发大量历史测试更新；这是本次目标，不做兼容缓冲。
- 同包 Go 测试迁移到根目录后，部分断言需要提升到 CLI 或业务行为层，执行阶段要避免机械搬文件导致覆盖变弱。
- 全仓库清理旧后端字样会触及历史归档材料；本提案接受该成本，以换取后续搜索和维护的清晰性。
