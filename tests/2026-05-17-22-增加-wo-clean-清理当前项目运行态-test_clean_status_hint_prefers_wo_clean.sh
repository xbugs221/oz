#!/usr/bin/env bash
# 22-增加-wo-clean-清理当前项目运行态: status 对不可恢复 batch 优先提示 wo clean
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT
WO="$TMPDIR/wo"
go build -C "$REPO_ROOT" -o "$WO" ./cmd/wo

export XDG_STATE_HOME="$TMPDIR/state"
REPO="$TMPDIR/repo"
mkdir -p "$REPO"
cd "$REPO"
git init >/dev/null
git config user.email "test@example.com"
git config user.name "Test"
echo "test" > README.md
git add . && git commit -m "init" >/dev/null

python3 - <<'PY'
import hashlib, json, os

repo = os.getcwd()
repo_key = os.path.basename(repo).lower().replace(".", "-") + "-" + hashlib.sha1(repo.encode()).hexdigest()[:10]
base = os.path.join(os.environ["XDG_STATE_HOME"], "wo", "repos", repo_key)
runs = os.path.join(base, "runs")
batches = os.path.join(base, "batches")

# Create a blocked run.
os.makedirs(os.path.join(runs, "run-blocked"), exist_ok=True)
with open(os.path.join(runs, "run-blocked", "state.json"), "w") as f:
    json.dump({
        "run_id": "run-blocked", "change_name": "1-a", "sealed": True,
        "status": "blocked_review_limit", "stage": "blocked_review_limit",
        "error": "审核修正达到上限，工作流已中断",
        "baseline_head": "abc123", "baseline_diff": "",
        "sessions": {}, "stages": {"execution": "completed", "review_1": "completed"},
        "paths": {}, "batch_id": "batch-blocked",
        "workflow_config": {"max_review_iterations": 3, "stages": {}}
    }, f, indent=2)

# Create a blocked batch that references the blocked run.
os.makedirs(os.path.join(batches, "batch-blocked"), exist_ok=True)
with open(os.path.join(batches, "batch-blocked", "state.json"), "w") as f:
    json.dump({
        "batch_id": "batch-blocked", "status": "failed",
        "changes": ["1-a", "2-b"], "current_index": 0,
        "run_ids": {"1-a": "run-blocked"},
        "failed_change": "1-a", "failed_run_id": "run-blocked",
        "error": "blocked"
    }, f, indent=2)
PY

STATUS_OUTPUT=$("$WO" status -b1 2>&1) || true
echo "=== wo status -b1 output ==="
echo "$STATUS_OUTPUT"

if ! echo "$STATUS_OUTPUT" | grep -q "wo clean"; then
  echo "FAIL: status output missing 'wo clean' hint"
  exit 1
fi
echo "OK: status output contains 'wo clean' hint"

if echo "$STATUS_OUTPUT" | grep -q "rm -rf"; then
  echo "FAIL: status output should not contain 'rm -rf' as primary cleanup command"
  exit 1
fi
echo "OK: status output does not contain 'rm -rf'"

if ! echo "$STATUS_OUTPUT" | grep -q "该操作仅删除 wo 历史记录"; then
  echo "FAIL: status output missing code-change disclaimer"
  exit 1
fi
echo "OK: code-change disclaimer present"

echo "PASS: status prefers wo clean hint for unrecoverable batch"
