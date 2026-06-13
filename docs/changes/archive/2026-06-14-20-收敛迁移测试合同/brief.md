# 文件目的

本提案收敛 `tests/app/*.gotest` 迁移测试合同，让根目录测试门禁重新代表当前源码真实意图。

## 用户问题

当前 `go test ./internal/app` 与 `go test ./cmd/oz` 可以通过，但 `go test ./...` 会在迁移测试包装层失败。失败来自历史 `.gotest` 合同与当前 go-dag 行为分叉，导致后续重构前无法判断测试失败是回归还是旧合同噪音。

## 交付目标

- 将仍有效的 `tests/app/*.gotest` 合同迁回真实 Go 测试包，或改写为当前意图一致的测试。
- 删除或清空过期 `.gotest` 迁移入口，根门禁不再通过临时复制包执行历史合同。
- 保持 `go test ./...` 可作为后续重构的稳定基线。

## 非目标

- 不修改 `wo` 工作流业务语义。
- 不拆分生产代码结构。
- 不弱化当前真实 go-dag、status、batch 行为测试。

## 验收入口

执行 `bash docs/changes/20-收敛迁移测试合同/tests/migrated-tests-contract_test.sh`。

## 执行阶段默认上下文

优先读取 `tests/app/migrated_app_suite_test.go`、`tests/app/*.gotest`、`internal/app/*_test.go` 和当前失败日志。先确认每个失败断言是否仍符合当前业务意图，再迁移或删除。
