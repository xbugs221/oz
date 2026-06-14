# 提案：拆分 ozcli 命令边界

## 背景

`ozcli.go` 是 standalone `oz` 命令的全部实现。随着 oz 提案校验、验收合同和归档规则变多，单文件维护成本已经超过收益。

## 目标

- 把 CLI 入口和命令分发保留在小文件。
- 把 install 命令独立。
- 把 list/create/status 等 change 查询命令独立。
- 把 validate 及其 acceptance/runtime artifact policy 独立。
- 把 archive 及编号/任务完成检查独立。
- 保持 `go test ./internal/ozcli` 全部通过。

## 非目标

- 不改变 `oz` 子命令语义。
- 不调整 `acceptance.json` 校验强度。
- 不修改归档目录命名规则。
