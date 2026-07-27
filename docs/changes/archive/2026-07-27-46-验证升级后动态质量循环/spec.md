# 规格：升级后动态质量循环验证

### 需求：安装版默认配置使用动态质量循环

升级后的真实安装版必须生成无固定轮次上限、且明确表达动态阶段与独立 QA 的默认配置。

#### 场景：升级后的安装版生成无固定轮次上限的动态质量循环配置

- **给定** 当前源码已安装为 `PATH` 中可执行的 `oz`
- **当** 在全新临时 Git 仓库执行 `oz flow config`
- **则** 生成的默认 `oz-flow.yaml` 不包含 `max_repair_iterations`
- **并且** 默认提示明确表达动态 `audit_N`、`targeted_repair_N` 和独立 QA 合同
- **测试**：`docs/changes/archive/2026-07-27-46-验证升级后动态质量循环/tests/test_installed_quality_loop.sh`
- **真实数据来源**：`PATH` 中的真实 `oz`、临时 Git 仓库及其生成的 `oz-flow.yaml`
- **入口路径**：`oz flow config`
- **关键断言**：真实安装版可生成配置；配置无固定轮次上限；动态阶段和独立 QA 提示存在
- **剩余风险**：模型实际审查质量不由配置文本单独证明
