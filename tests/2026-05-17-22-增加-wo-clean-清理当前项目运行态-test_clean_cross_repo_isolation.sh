#!/usr/bin/env bash
# 22-增加-wo-clean-清理当前项目运行态: 两端隔离 - repo A clean 不影响 repo B
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT
WO="$TMPDIR/wo"
go build -C "$REPO_ROOT" -o "$WO" ./cmd/wo

export XDG_STATE_HOME="$TMPDIR/state"

# Create repo A with a failed run.
REPO_A="$TMPDIR/repo-a"
mkdir -p "$REPO_A"
cd "$REPO_A"
git init >/dev/null
git config user.email "test@example.com"
git config user.name "Test"
echo "a" > README.md
git add . && git commit -m "init" >/dev/null

python3 - "$REPO_A" "run-failed-a" "failed" <<'PY'
import hashlib, json, os, sys
repo = sys.argv[1]
run_id = sys.argv[2]
status = sys.argv[3]
repo_key = os.path.basename(repo).lower().replace(".", "-") + "-" + hashlib.sha1(repo.encode()).hexdigest()[:10]
base = os.path.join(os.environ["XDG_STATE_HOME"], "wo", "repos", repo_key)
runs = os.path.join(base, "runs")
os.makedirs(os.path.join(runs, run_id), exist_ok=True)
state = {
    "run_id": run_id, "change_name": "1-演示", "sealed": True,
    "status": status, "stage": "execution", "error": "",
    "baseline_head": "abc123", "baseline_diff": "",
    "sessions": {}, "stages": {}, "paths": {},
    "workflow_config": {"max_review_iterations": 3, "stages": {}}
}
with open(os.path.join(runs, run_id, "state.json"), "w") as f:
    json.dump(state, f, indent=2)
PY

# Create repo B with a failed run.
REPO_B="$TMPDIR/repo-b"
mkdir -p "$REPO_B"
cd "$REPO_B"
git init >/dev/null
git config user.email "test@example.com"
git config user.name "Test"
echo "b" > README.md
git add . && git commit -m "init" >/dev/null

python3 - "$REPO_B" "run-failed-b" "failed" <<'PY'
import hashlib, json, os, sys
repo = sys.argv[1]
run_id = sys.argv[2]
status = sys.argv[3]
repo_key = os.path.basename(repo).lower().replace(".", "-") + "-" + hashlib.sha1(repo.encode()).hexdigest()[:10]
base = os.path.join(os.environ["XDG_STATE_HOME"], "wo", "repos", repo_key)
runs = os.path.join(base, "runs")
os.makedirs(os.path.join(runs, run_id), exist_ok=True)
state = {
    "run_id": run_id, "change_name": "1-演示", "sealed": True,
    "status": status, "stage": "execution", "error": "",
    "baseline_head": "abc123", "baseline_diff": "",
    "sessions": {}, "stages": {}, "paths": {},
    "workflow_config": {"max_review_iterations": 3, "stages": {}}
}
with open(os.path.join(runs, run_id, "state.json"), "w") as f:
    json.dump(state, f, indent=2)
PY

# Run wo clean in repo A.
cd "$REPO_A"
OUTPUT=$("$WO" clean 2>&1) || true
echo "=== wo clean output (repo A) ==="
echo "$OUTPUT"

# Verify repo A's failed run is cleaned.
if python3 -c "
import hashlib, os
repo = '$REPO_A'
repo_key = os.path.basename(repo).lower().replace('.', '-') + '-' + hashlib.sha1(repo.encode()).hexdigest()[:10]
run_path = os.path.join(os.environ['XDG_STATE_HOME'], 'wo', 'repos', repo_key, 'runs', 'run-failed-a')
print('exists' if os.path.exists(run_path) else 'cleaned')
" | grep -q "cleaned"; then
  echo "OK: repo A's failed run cleaned"
else
  echo "FAIL: repo A's failed run NOT cleaned"
  exit 1
fi

# Verify repo B's failed run is still present.
if python3 -c "
import hashlib, os
repo = '$REPO_B'
repo_key = os.path.basename(repo).lower().replace('.', '-') + '-' + hashlib.sha1(repo.encode()).hexdigest()[:10]
run_path = os.path.join(os.environ['XDG_STATE_HOME'], 'wo', 'repos', repo_key, 'runs', 'run-failed-b')
print('exists' if os.path.exists(run_path) else 'cleaned')
" | grep -q "exists"; then
  echo "OK: repo B's failed run preserved"
else
  echo "FAIL: repo B's failed run was accidentally cleaned"
  exit 1
fi

if ! echo "$OUTPUT" | grep -q "已清理"; then
  echo "FAIL: output missing clean count"
  exit 1
fi
echo "OK: Chinese output present"

echo "PASS: wo clean only affected current repo"
