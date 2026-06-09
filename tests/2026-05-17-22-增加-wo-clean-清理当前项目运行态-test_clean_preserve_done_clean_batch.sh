#!/usr/bin/env bash
# 22-增加-wo-clean-清理当前项目运行态: 保留 done 历史，清理 failed batch 及引用 run
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

python3 - "$REPO" <<'PY'
import hashlib, json, os

repo = os.getcwd()
repo_key = os.path.basename(repo).lower().replace(".", "-") + "-" + hashlib.sha1(repo.encode()).hexdigest()[:10]
base = os.path.join(os.environ["XDG_STATE_HOME"], "wo", "repos", repo_key)
runs = os.path.join(base, "runs")
batches = os.path.join(base, "batches")

# Create a done run (should be preserved).
os.makedirs(os.path.join(runs, "run-done"), exist_ok=True)
with open(os.path.join(runs, "run-done", "state.json"), "w") as f:
    json.dump({"run_id": "run-done", "change_name": "1-演示", "sealed": True,
               "status": "done", "stage": "done", "error": "",
               "baseline_head": "abc123", "baseline_diff": "",
               "sessions": {}, "stages": {}, "paths": {},
               "workflow_config": {"max_review_iterations": 3, "stages": {}}}, f, indent=2)

# Create a standalone failed run.
os.makedirs(os.path.join(runs, "run-failed-standalone"), exist_ok=True)
with open(os.path.join(runs, "run-failed-standalone", "state.json"), "w") as f:
    json.dump({"run_id": "run-failed-standalone", "change_name": "1-b", "sealed": True,
               "status": "failed", "stage": "execution", "error": "",
               "baseline_head": "abc123", "baseline_diff": "",
               "sessions": {}, "stages": {}, "paths": {},
               "workflow_config": {"max_review_iterations": 3, "stages": {}}}, f, indent=2)

# Create a failed run referenced by the batch.
os.makedirs(os.path.join(runs, "run-failed-batch-ref"), exist_ok=True)
with open(os.path.join(runs, "run-failed-batch-ref", "state.json"), "w") as f:
    json.dump({"run_id": "run-failed-batch-ref", "change_name": "2-b", "sealed": True,
               "status": "failed", "stage": "execution", "error": "",
               "baseline_head": "abc123", "baseline_diff": "",
               "sessions": {}, "stages": {}, "paths": {},
               "workflow_config": {"max_review_iterations": 3, "stages": {}}}, f, indent=2)

# Create a failed batch referencing run-failed-batch-ref.
os.makedirs(os.path.join(batches, "batch-failed"), exist_ok=True)
with open(os.path.join(batches, "batch-failed", "state.json"), "w") as f:
    json.dump({"batch_id": "batch-failed", "status": "failed",
               "changes": ["1-a", "2-b"], "current_index": 1,
               "run_ids": {"1-a": "run-done", "2-b": "run-failed-batch-ref"},
               "failed_change": "2-b", "failed_run_id": "run-failed-batch-ref",
               "error": "failed"}, f, indent=2)
PY

OUTPUT=$("$WO" clean 2>&1) || true
echo "=== wo clean output ==="
echo "$OUTPUT"

# Verify done run is preserved.
if python3 -c "
import hashlib, os
repo = '$REPO'
repo_key = os.path.basename(repo).lower().replace('.', '-') + '-' + hashlib.sha1(repo.encode()).hexdigest()[:10]
path = os.path.join(os.environ['XDG_STATE_HOME'], 'wo', 'repos', repo_key, 'runs', 'run-done')
print('exists' if os.path.exists(path) else 'cleaned')
" | grep -q "exists"; then
  echo "OK: done run preserved"
else
  echo "FAIL: done run was cleaned"
  exit 1
fi

# Verify failed batch is cleaned.
if python3 -c "
import hashlib, os
repo = '$REPO'
repo_key = os.path.basename(repo).lower().replace('.', '-') + '-' + hashlib.sha1(repo.encode()).hexdigest()[:10]
path = os.path.join(os.environ['XDG_STATE_HOME'], 'wo', 'repos', repo_key, 'batches', 'batch-failed')
print('exists' if os.path.exists(path) else 'cleaned')
" | grep -q "cleaned"; then
  echo "OK: failed batch cleaned"
else
  echo "FAIL: failed batch NOT cleaned"
  exit 1
fi

# Verify batch-referenced failed run is cleaned.
if python3 -c "
import hashlib, os
repo = '$REPO'
repo_key = os.path.basename(repo).lower().replace('.', '-') + '-' + hashlib.sha1(repo.encode()).hexdigest()[:10]
path = os.path.join(os.environ['XDG_STATE_HOME'], 'wo', 'repos', repo_key, 'runs', 'run-failed-batch-ref')
print('exists' if os.path.exists(path) else 'cleaned')
" | grep -q "cleaned"; then
  echo "OK: batch-referenced failed run cleaned"
else
  echo "FAIL: batch-referenced failed run NOT cleaned"
  exit 1
fi

# Verify standalone failed run is cleaned.
if python3 -c "
import hashlib, os
repo = '$REPO'
repo_key = os.path.basename(repo).lower().replace('.', '-') + '-' + hashlib.sha1(repo.encode()).hexdigest()[:10]
path = os.path.join(os.environ['XDG_STATE_HOME'], 'wo', 'repos', repo_key, 'runs', 'run-failed-standalone')
print('exists' if os.path.exists(path) else 'cleaned')
" | grep -q "cleaned"; then
  echo "OK: standalone failed run cleaned"
else
  echo "FAIL: standalone failed run NOT cleaned"
  exit 1
fi

if ! echo "$OUTPUT" | grep -q "已清理 1 个批量任务"; then
  echo "FAIL: output missing batch count"
  exit 1
fi
echo "OK: correct counts in output"

echo "PASS: preserve done history and clean failed batch with referenced runs"
