#!/usr/bin/env bash
set -euo pipefail

# 验证 running batch 与 running single-run 同时存在时，wo watch 优先展示 batch。

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

# Running batch.
batch_id = "20260517T040000.500000000Z"
batch_run_id = "20260517T040000.000000000Z"
write_json(os.path.join(runs, batch_run_id, "state.json"), {
    "run_id": batch_run_id,
    "change_name": "1-batch变更",
    "sealed": True,
    "status": "running",
    "stage": "execution",
    "baseline_head": "abc",
    "baseline_diff": "",
    "batch_id": batch_id,
    "sessions": {},
    "stages": {},
    "paths": {},
    "workflow_config": workflow,
})
write_json(os.path.join(batches, batch_id, "state.json"), {
    "batch_id": batch_id,
    "status": "running",
    "changes": ["1-batch变更", "2-下一变更"],
    "current_index": 0,
    "run_ids": {"1-batch变更": batch_run_id},
})

# Also a running single-run.
single_run_id = "20260517T040001.000000000Z"
write_json(os.path.join(runs, single_run_id, "state.json"), {
    "run_id": single_run_id,
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

# Run watch with timeout. Since it's non-TTY, it will output one frame.
# In non-TTY mode, the first frame is rendered immediately, then a ticker fires.
# Use timeout to capture the initial frame.
timeout -s INT 2s "$WO" watch > watch_output.txt 2>&1 || true

# Watch should show compact batch proposal content, not single-run.
grep -qF -- "- 1-batch变更" watch_output.txt
grep -qF -- "- 2-下一变更" watch_output.txt

# Only one frame expected in non-TTY (might get 2-3 frames from ticker).
# Verify it's showing batch not single-run in the first line.
head -1 watch_output.txt | grep -qF -- "- 1-batch变更"
! grep -qF "| b1 1/2" watch_output.txt
! grep -qF "工作流 w1 running" watch_output.txt

echo "PASS"
