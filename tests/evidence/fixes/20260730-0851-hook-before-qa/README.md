# 独立 QA 前稳定提交钩子

## 根因

原 repair 与 archive 合同要求测试和归档保持只读，但没有规定提交前钩子的执行时机。钩子直到归档 commit 才首次运行时，会在最终 QA 后改变 tracked source，使归档快照正确地判定漂移。

## 修复

- `oz-exec` 与动态 repair 合同要求在每次独立 QA 前显式运行提交前钩子。
- 第一次改动被吸收后必须再次运行，第二次无改动才算稳定。
- 稳定后重新运行受影响测试、required tests 和 validation commands。
- archive 合同禁止最终 QA 后首次触发改写；仍有改动时停止归档并返回自查与 QA。
- 动态质量循环追加合同同步更新，使升级后的旧 sealed prompt 快照也能收到该约束。

## 验证

- Go 合同覆盖内置提示、仓库配置提示和动态追加提示。
- shell 合同覆盖 `oz-exec`、`oz-archive` 与 README 的公开职责说明。
