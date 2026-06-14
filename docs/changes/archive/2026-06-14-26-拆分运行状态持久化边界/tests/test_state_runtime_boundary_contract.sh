#!/usr/bin/env bash
# 文件功能目的：验证 sealed run 状态持久化、prompt 上下文和 git 守卫已经从 state.go 拆成稳定边界。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
log="$repo_root/test-results/refactor-state-runtime-boundary/state-runtime-boundary-contract.log"
mkdir -p "$(dirname "$log")"
: >"$log"

note() {
  # note 记录关键步骤，同时产出 state-runtime-boundary-log 证据。
  printf '%s\n' "$*" | tee -a "$log"
}

fail() {
  # fail 使用业务语义说明失败点，避免执行阶段只看到 shell 行号。
  note "FAIL: $*"
  exit 1
}

cd "$repo_root"

note "evidence id: state-runtime-boundary-log"
note "evidence path: $log"
note "test id: state-runtime-boundary-contract"

for file in \
  internal/app/state_store.go \
  internal/app/run_lock.go \
  internal/app/prompt_context.go \
  internal/app/git_guard.go
do
  [[ -f "$file" ]] || fail "缺少运行状态边界文件：$file"
  note "已发现边界文件：$file"
done

for symbol in \
  'func saveState' \
  'func loadState' \
  'func mergeState' \
  'func promptForStage' \
  'func promptContext' \
  'func gitSnapshot' \
  'func classifyGitSnapshotChange' \
  'func acquireLock' \
  'func lockFileStatus'
do
  if rg -n "$symbol" internal/app/state.go | tee -a "$log" | grep -q .; then
    fail "state.go 仍直接定义已拆分职责：$symbol"
  fi
done

note "运行状态机、Go DAG、人工干预和 acceptance preflight 回归"
go test ./internal/app \
  -run 'TestEngineStartRunsCleanReviewsToDone|TestQAFailureReturnsToFix|TestDetectManualIntervention|TestGoDAGRetryableHelperErrorRestoresRunningState|TestRootAcceptancePreflight' \
  -count=1 2>&1 | tee -a "$log"

note "PASS: state-runtime-boundary-contract"
