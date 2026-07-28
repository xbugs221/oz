<!-- 文件目的：记录 QA 因验收证据哈希变化错误暂停的根因、修复和验证。 -->

# QA 验收证据哈希变化错误暂停

## 场景与证据

动态质量循环在 audit 已通过后进入 QA。若 required evidence 在 QA 前或 QA 期间产生了新内容，持久 checkpoint 的 evidence hash 与当前 hash 不同，运行被置为 `blocked_stalled`，必须人工执行 `oz flow restart`。

## 根因

置信度：Confirmed。

`prepareQualityLoopQAReadOnlyGate` 与 `verifyQualityLoopQAReadOnlyGate` 把两类情况都交给 `blockQualityLoopQAReadOnly`：

- 合法普通文件的内容发生变化，旧审核结论已经过期；
- 证据缺失、变成目录/FIFO/设备或 checkpoint 被篡改。

前者代表有新进展，应重新审核；只有后者需要安全暂停。

## 修复

- 为 acceptance tests/evidence 内容哈希漂移增加类型化错误。
- QA 前或 QA 期间发现合法内容漂移时，自动切换到新的 `audit_N`。
- 清理未受信 QA 的运行态绑定，保留旧 audit 证据供追溯。
- 缺失或不安全 evidence、checkpoint 篡改和 QA 越权改源码继续暂停。

## 回归测试

- required evidence 内容在 QA 前变化：不调用旧 QA，自动进入 `audit_2`。
- required evidence 内容在 QA 期间变化：旧 QA 不获信任，自动进入 `audit_2`。
- evidence 变为目录、FIFO 或设备：继续进入 `blocked_stalled`。

## 剩余风险

自动重审仍可能发现真实缺陷并进入定向修复，这是正常质量循环，不应被视为流程中断。
