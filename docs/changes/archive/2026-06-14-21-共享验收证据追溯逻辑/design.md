# 文件目的

本文件记录共享 producer 追溯逻辑的设计决策。

## 技术方案

在 `internal/acceptance` 增加纯业务 API，例如：

```go
func ProducerFindings(projectRoot string, contract Contract) []string
func EvidenceHasProducer(projectRoot string, evidence Evidence, coverage []Coverage, tests map[string]Test) bool
```

入口层职责：

- `cmd/oz` 负责把 `docs` 根路径转换为项目根路径，并把 findings 转成 validate 错误。
- `internal/app` 负责读取 sealed acceptance，并把 findings 写入 `AcceptancePreflightState`。

## 取舍

共享包可以读取 repo-relative 测试文件和 wrapper，这是为了保持现有 producer 追溯能力。暂不引入命令行 AST 或 shell parser，因为当前规则只需要保守字符串追踪。

## 风险

- 错误文案可能变化。缓解方式是在入口层保留用户可理解的中文上下文。
- 路径根传递错误会造成误报。缓解方式是共享包测试覆盖项目根、docs 根和 sibling wrapper。
