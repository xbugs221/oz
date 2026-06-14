#!/usr/bin/env bash
# 文件功能：验证子智能体执行路径被拆成 retry、boundary、artifact、prompt 四个稳定边界。

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
cd "$ROOT"

EVIDENCE="test-results/33-subagent-boundary/contract.log"
mkdir -p "$(dirname "$EVIDENCE")"
: > "$EVIDENCE"

note() {
  printf '%s\n' "$*" | tee -a "$EVIDENCE"
}

fail() {
  note "FAIL: $*"
  exit 1
}

assert_file_has() {
  local file="$1"
  local pattern="$2"
  [[ -f "$file" ]] || fail "缺少目标文件：$file"
  rg -n "$pattern" "$file" >>"$EVIDENCE" || fail "$file 缺少模式：$pattern"
}

note "subagent-boundary-log: 检查 subagent 文件边界"
assert_file_has "internal/app/subagent_attempt.go" 'func \(e \*Engine\) runSubagentAttempt'
assert_file_has "internal/app/subagent_attempt.go" 'runner\.Run|artifactRetryPrompt|errGoDAGRetryableNode'
assert_file_has "internal/app/subagent_boundary.go" 'func \(e \*Engine\) checkSubagentReadOnlyBoundary|func classifyRunArtifactChanges'
assert_file_has "internal/app/subagent_artifact.go" 'func readMemberArtifact|func writeMemberArtifact|func materializeCapturedMemberArtifact'
assert_file_has "internal/app/subagent_prompt.go" 'func subagentPromptContext|func subagentPrompt|func artifactRetryPrompt'

if [[ -f internal/app/subagent.go ]]; then
  line_count="$(wc -l < internal/app/subagent.go | tr -d ' ')"
  note "subagent.go line_count=$line_count"
  (( line_count <= 260 )) || fail "subagent.go 仍然过大，说明执行边界没有真正拆分"
fi

note "运行 internal/app subagent/parallel 相关 Go 回归"
go test ./internal/app -run 'Test(Subagent|Parallel|MemberArtifact|GoDAGSubagents)' -count=1 | tee -a "$EVIDENCE"

note "contract passed: subagent 执行边界已拆分，证据位于 $EVIDENCE"
