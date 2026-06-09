#!/usr/bin/env bash
set -euo pipefail

# 验证真实 wo status 在 batch 场景下输出精简队列树，并保留单 run/JSON 查询能力。

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
run_id = "20260512T051106.925886354Z"
write_json(os.path.join(runs, run_id, "state.json"), {
    "run_id": run_id,
    "change_name": "45-智能体文件写入保护",
    "sealed": True,
    "status": "running",
    "stage": "review_1",
    "baseline_head": "abc",
    "baseline_diff": "",
    "batch_id": "20260512T051106.910247319Z",
    "sessions": {"codex:reviewer": "019e1a9c-973b-7a31-b787-f5268bc033c1"},
    "stages": {"execution": "completed"},
    "paths": {},
    "workflow_config": workflow,
    "error": "",
})
write_json(os.path.join(batches, "20260512T051106.910247319Z", "state.json"), {
    "batch_id": "20260512T051106.910247319Z",
    "status": "running",
    "changes": ["45-智能体文件写入保护", "46-重构主从智能体文件树", "47-智能体命令执行deny策略"],
    "current_index": 0,
    "run_ids": {"45-智能体文件写入保护": run_id},
    "error": "",
})
PY

WO="$TMPDIR/wo"
go build -C "$REPO_ROOT" -o "$WO" ./cmd/wo

"$WO" status > status.txt
grep -qF "→ b1 1/3" status.txt
grep -qF -- "- 45-智能体文件写入保护" status.txt
grep -qF -- "- 46-重构主从智能体文件树" status.txt
grep -qF -- "- 47-智能体命令执行deny策略" status.txt
grep -qF "  执行阶段 - ✓ -" status.txt
grep -qF "  审核阶段 019e1a9c-973b-7a31-b787-f5268bc033c1 → -" status.txt
! grep -qF "20260512T051106.910247319Z" status.txt
! grep -qF "20260512T051106.925886354Z" status.txt
! grep -qF "工作流 w" status.txt
! grep -qF "批量任务 b1 running" status.txt
! grep -qF "未开始" status.txt

"$WO" status -w1 > status-w1.txt
grep -qF "执行阶段 - ✓ -" status-w1.txt
! grep -qF "批量任务" status-w1.txt

"$WO" status --run-id 20260512T051106.925886354Z --json > status.json
grep -qF '"run_id":"20260512T051106.925886354Z"' status.json
grep -qF '"sessions"' status.json
! grep -qF "批量任务" status.json
! grep -qF "工作流" status.json

echo "PASS"
