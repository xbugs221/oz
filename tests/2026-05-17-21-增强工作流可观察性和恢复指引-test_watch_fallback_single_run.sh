#!/usr/bin/env bash
set -euo pipefail

# 验证没有 running batch 时，wo watch 回退展示 single-run。

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

export XDG_STATE_HOME="$TMPDIR/state"
REPO="$TMPDIR/repo"
mkdir -p "$REPO"
cd "$REPO"
git init >/dev/null

python3 - <<'PY'
import hashlib
import json
import os

repo_path = os.getcwd()
repo_key = os.path.basename(repo_path).lower().replace(".", "-") + "-" + hashlib.sha1(repo_path.encode()).hexdigest()[:10]
base = os.path.join(os.environ["XDG_STATE_HOME"], "wo", "repos", repo_key)
runs = os.path.join(base, "runs")
batches = os.path.join(base, "batches")
os.makedirs(runs, exist_ok=True)
os.makedirs(batches, exist_ok=True)

def write_json(path, data):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False)

workflow = {"max_review_iterations": 1, "stages": {}, "prompts": {}, "validation": {"commands": []}}

# Only a running single-run, no batch.
run_id = "20260517T050000.000000000Z"
write_json(os.path.join(runs, run_id, "state.json"), {
    "run_id": run_id,
    "change_name": "x-独立变更",
    "sealed": True,
    "status": "running",
    "stage": "execution",
    "baseline_head": "abc",
    "baseline_diff": "",
    "sessions": {},
    "stages": {},
    "paths": {},
    "workflow_config": workflow,
})
PY

WO="$TMPDIR/wo"
go build -C "$REPO_ROOT" -o "$WO" ./cmd/wo

# Run watch with timeout.
timeout -s INT 2s "$WO" watch > watch_output.txt 2>&1 || true

# Watch should show single-run content.
grep -qF "工作流 w1 running" watch_output.txt

echo "PASS"
