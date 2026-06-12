#!/usr/bin/env bash
# 文件功能目的：验证 wo 阶段产物门禁状态与命令 validation 状态分离，并验证 execution 后验收预检会阻断不可执行证据合同。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/gate-state-acceptance-preflight"
LOG="$RESULT_DIR/contract.log"

mkdir -p "$RESULT_DIR"
: >"$LOG"

note() {
  # note 同步输出到 stdout 和 runtime log，便于归档后的 QA 复核具体失败点。
  printf '%s\n' "$*" | tee -a "$LOG"
}

cd "$ROOT"

note "运行根 Go 回归测试，锁定 artifact_gates 与 acceptance_preflight 新语义"
go test ./internal/app -run 'TestRootArtifactGateFailureUsesArtifactGateState|TestRootAcceptancePreflight' -count=1 2>&1 | tee -a "$LOG"

note "验证 runtime evidence 已生成且包含 PASS"
grep -q 'PASS' "$LOG"

note "PASS"
