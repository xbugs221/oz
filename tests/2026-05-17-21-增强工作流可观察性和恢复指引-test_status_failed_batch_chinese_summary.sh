#!/usr/bin/env bash
set -euo pipefail

# 验证 failed batch 的 wo status 显示中文失败摘要和 restart 提示，而不是仅显示 "failed"。

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
run_id = "20260517T010000.000000000Z"
batch_id = "20260517T010000.500000000Z"

write_json(os.path.join(runs, run_id, "state.json"), {
    "run_id": run_id,
    "change_name": "1-演示变更",
    "sealed": True,
    "status": "failed",
    "stage": "execution",
    "baseline_head": "abc",
    "baseline_diff": "",
    "batch_id": batch_id,
    "sessions": {},
    "stages": {},
    "paths": {},
    "workflow_config": workflow,
    "error": "agent execution failed",
})
write_json(os.path.join(batches, batch_id, "state.json"), {
    "batch_id": batch_id,
    "status": "failed",
    "changes": ["1-演示变更", "2-下一变更"],
    "current_index": 0,
    "run_ids": {"1-演示变更": run_id},
    "failed_change": "1-演示变更",
    "failed_run_id": run_id,
    "error": "agent execution failed",
})
PY

WO="$TMPDIR/wo"
go build -C "$REPO_ROOT" -o "$WO" ./cmd/wo

# Check that status shows Chinese summary with change name and stage role.
"$WO" status > status.txt

# Must start directly from the proposal list without a batch header.
head -1 status.txt | grep -qF -- "- 1-演示变更"
! grep -qF "→ b1 1/2" status.txt

# Must show Chinese failure summary with change name and stage role.
grep -qF "1-演示变更" status.txt
grep -qF "写阶段失败" status.txt

# Must show restart hint with delete-and-continue wording.
grep -qF "wo restart -b1 删除失败记录并继续该批量任务" status.txt

# Must NOT show bare "failed" as the only reason.
! grep -qF "错误: failed" status.txt

echo "PASS"
