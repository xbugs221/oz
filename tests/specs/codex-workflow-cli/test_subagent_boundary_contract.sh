#!/usr/bin/env bash
# Sources: 33-拆分子智能体执行边界
# 文件功能目的：验证 subagent 执行路径被拆成 retry、boundary、artifact、prompt 四个稳定边界。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/specs/subagent-boundary"
EVIDENCE="$RESULT_DIR/contract.log"

mkdir -p "$RESULT_DIR"
: >"$EVIDENCE"

note() {
  # note 记录规格测试步骤，便于失败时定位是结构边界还是 Go 回归破坏。
  printf '%s\n' "$*" | tee -a "$EVIDENCE"
}

fail() {
  # fail 同时写入证据日志和 stderr，保留可复核的失败原因。
  note "FAIL: $*"
  exit 1
}

assert_file_has() {
  # assert_file_has 证明指定业务职责仍位于预期的生产代码边界文件中。
  local file="$1"
  local pattern="$2"
  [[ -f "$file" ]] || fail "缺少目标文件：$file"
  rg -n "$pattern" "$file" >>"$EVIDENCE" || fail "$file 缺少模式：$pattern"
}

cd "$ROOT"

note "检查 subagent 执行边界文件"
assert_file_has "internal/app/subagent_attempt.go" 'func \(e \*Engine\) runSubagentAttempts'
assert_file_has "internal/app/subagent_attempt.go" 'func \(e \*Engine\) runSubagentAttempt'
assert_file_has "internal/app/subagent_attempt.go" 'runner\.Run|artifactRetryPrompt|errGoDAGRetryableNode'
assert_file_has "internal/app/subagent_boundary.go" 'func \(e \*Engine\) checkSubagentReadOnlyBoundary|func classifyRunArtifactChanges'
assert_file_has "internal/app/subagent_artifact.go" 'func readMemberArtifact|func writeMemberArtifact|func materializeCapturedMemberArtifact'
assert_file_has "internal/app/subagent_prompt.go" 'func subagentPromptContext|func subagentPrompt|func artifactRetryPrompt'

line_count="$(wc -l < internal/app/subagent.go | tr -d ' ')"
note "subagent.go line_count=$line_count"
(( line_count <= 260 )) || fail "subagent.go 仍然过大，说明执行边界没有真正拆分"

if rg -n 'for attempt :=|runner\.Run|errGoDAGRetryableNode' internal/app/subagent.go >>"$EVIDENCE"; then
  fail "subagent.go 重新承载了 retry/attempt 执行细节"
fi

note "运行 internal/app subagent/parallel 相关 Go 回归"
go test ./internal/app -run 'Test(Subagent|Parallel|MemberArtifact|GoDAGSubagents)' -count=1 2>&1 | tee -a "$EVIDENCE"

note "PASS: subagent 执行边界已拆分，证据位于 $EVIDENCE"
