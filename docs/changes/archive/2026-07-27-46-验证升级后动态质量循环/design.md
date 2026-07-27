# 设计：真实安装版动态质量循环验证

## 验证链路

```text
当前本地源码
  → 安装 PATH 中的 oz
  → 临时 Git 仓库执行 oz flow config
  → 检查生成的 oz-flow.yaml
  → 写入本地 runtime.log
```

合同测试直接解析真实安装版生成的配置，不调用测试专用入口，也不伪造配置内容。

## 配置合同

默认配置必须同时满足：

1. 不输出已弃用的 `max_repair_iterations`。
2. repair 提示明确说明 `pre_qa_audit` 对应动态 `audit_N`。
3. repair 提示明确说明 `qa_targeted_repair` 对应动态 `targeted_repair_N`。
4. QA 仍由独立会话放行。

实现只补充提示词中的阶段映射，不改变动态调度、状态恢复或旧 sealed run 兼容逻辑。

## 证据与风险

运行证据写入 `test-results/46-installed-quality-loop/runtime.log`，仅供当前 QA 复核，不进入 Git。配置合同能证明安装内容正确，但模型实际审查质量仍由独立 QA 观察。

