#!/usr/bin/env bash
set -euo pipefail

# 验证不可恢复 batch (aborted) 的 wo status 显示整条历史的 rm -rf 清理命令。

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
run_id = "20260517T020000.000000000Z"
batch_id = "20260517T020000.500000000Z"

write_json(os.path.join(runs, run_id, "state.json"), {
    "run_id": run_id,
    "change_name": "1-中止变更",
    "sealed": True,
    "status": "aborted_manual_intervention",
    "stage": "execution",
    "baseline_head": "abc",
    "baseline_diff": "",
    "batch_id": batch_id,
    "sessions": {},
    "stages": {},
    "paths": {},
    "workflow_config": workflow,
    "error": "用户已中止",
})
write_json(os.path.join(batches, batch_id, "state.json"), {
    "batch_id": batch_id,
    "status": "aborted",
    "changes": ["1-中止变更", "2-下一变更"],
    "current_index": 0,
    "run_ids": {"1-中止变更": run_id},
    "error": "用户已中止",
})
PY

WO="$TMPDIR/wo"
go build -C "$REPO_ROOT" -o "$WO" ./cmd/wo

"$WO" status > status.txt

# Must show wo clean cleanup command.
grep -qF "wo clean" status.txt

# Must explain that cleanup only removes wo history, not code changes.
grep -qF "该操作仅删除 wo 历史记录" status.txt

# Must not show restart hint for aborted batch.
! grep -qF "wo restart -b1" status.txt

echo "PASS"
