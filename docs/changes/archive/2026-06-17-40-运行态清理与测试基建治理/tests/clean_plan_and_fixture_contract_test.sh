#!/usr/bin/env bash
# 文件功能目的：验证 clean plan/apply 安全边界和 workflow 测试夹具已经形成。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/40-clean-plan-fixture"
LOG="$RESULT_DIR/contract.log"
TMP_DIR="$RESULT_DIR/tmp"
OZ_BIN="$RESULT_DIR/oz"

rm -rf "$TMP_DIR"
mkdir -p "$RESULT_DIR" "$TMP_DIR"
: >"$LOG"

note() {
  printf '[clean-plan-fixture] %s\n' "$*" | tee -a "$LOG"
}

fail() {
  printf '[clean-plan-fixture] FAIL: %s\n' "$*" | tee -a "$LOG" >&2
  exit 1
}

assert_rg() {
  local pattern="$1"
  local path="$2"
  local message="$3"
  if ! rg -n "$pattern" "$path" >>"$LOG" 2>&1; then
    fail "$message"
  fi
}

note "检查 clean plan/apply 和 workflow fixture 边界"
assert_rg 'type CleanPlan|func BuildCleanPlan|func ApplyCleanPlan' "$ROOT/internal/app" "缺少 clean plan/apply 边界"
assert_rg 'workflowFixture|newWorkflowFixture|fakeWorkflowRunner' "$ROOT/internal/app" "缺少共享 workflow 测试夹具"

note "构建真实 oz 二进制"
(cd "$ROOT" && go build -o "$OZ_BIN" ./cmd/oz) >>"$LOG" 2>&1 || fail "go build ./cmd/oz 失败"

PROJECT="$TMP_DIR/project"
STATE_HOME="$TMP_DIR/state"
mkdir -p "$PROJECT" "$STATE_HOME"
cd "$PROJECT"
git init >>"$LOG" 2>&1
git config user.email "oz-test@example.com"
git config user.name "oz test"
printf 'demo\n' > README.md
git add README.md >>"$LOG" 2>&1
git commit -m init >>"$LOG" 2>&1

repo_key="$(python3 - "$PROJECT" <<'PY'
import hashlib
import os
import re
import sys

path = os.path.abspath(sys.argv[1])
name = os.path.basename(os.path.normpath(path)).lower()
name = re.sub(r'[^a-z0-9]+', '-', name).strip('-') or 'repo'
print(f"{name}-{hashlib.sha1(path.encode()).hexdigest()[:10]}")
PY
)"

RUN_DIR="$STATE_HOME/oz/flow/repos/$repo_key/runs/run-failed"
mkdir -p "$RUN_DIR"
cat >"$RUN_DIR/state.json" <<'EOF'
{
  "run_id": "run-failed",
  "change_name": "1-clean-plan",
  "sealed": true,
  "status": "failed",
  "stage": "execution",
  "error": "contract fixture failure",
  "sessions": {},
  "stages": {},
  "paths": {},
  "workflow_config": {}
}
EOF

note "运行 dry-run JSON，确认只生成计划不删除"
XDG_STATE_HOME="$STATE_HOME" "$OZ_BIN" flow clean --dry-run --json >"$RESULT_DIR/clean-plan.json" 2>>"$LOG" || fail "oz flow clean --dry-run --json 应成功"
[[ -d "$RUN_DIR" ]] || fail "dry-run 不得删除 failed run 目录"
if ! rg -n 'run-failed|delete|would_delete|clean_plan|dry_run' "$RESULT_DIR/clean-plan.json" >>"$LOG" 2>&1; then
  fail "dry-run JSON 未包含 failed run 删除计划"
fi

note "运行实际 clean，确认同一 failed run 被删除"
XDG_STATE_HOME="$STATE_HOME" "$OZ_BIN" flow clean >>"$LOG" 2>&1 || fail "oz flow clean 应成功 apply 计划"
[[ ! -e "$RUN_DIR" ]] || fail "实际 clean 后 failed run 目录仍存在"

note "运行 internal/app 回归测试"
(cd "$ROOT" && go test ./internal/app) >>"$LOG" 2>&1 || fail "go test ./internal/app 失败"

note "PASS: clean plan and workflow fixture contract"
