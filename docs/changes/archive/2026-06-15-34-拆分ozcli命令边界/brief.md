# 34-拆分ozcli命令边界

当前 `internal/ozcli/ozcli.go` 把 standalone `oz` CLI 的 install、list、create、status、validate、archive、版本解析和工具函数放在一个文件里。`validate` 与 `archive` 已经是较重的业务逻辑，继续集中会增加后续改 oz 提案合同的风险。

本次交付目标是按命令职责拆分 `internal/ozcli`，保持当前命令输出、JSON 合同和归档校验行为不变。非目标是不改 `oz` 命令集合、不引入新 CLI 框架、不改变 acceptance 规则。

执行阶段默认先运行 `bash docs/changes/34-拆分ozcli命令边界/tests/ozcli_boundary_test.sh`，确认当前实现失败于结构边界缺失，再完成拆分并让测试通过。验收证据写入 `test-results/34-ozcli-boundary/contract.log`。
