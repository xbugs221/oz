#!/usr/bin/env bash
set -euo pipefail

# 验证真实 wo status 在 failed batch 场景下隐藏 agent 后端网络诊断，同时保留 JSON 查询契约。

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
run_id = "20260513T011353.638549214Z"
batch_id = "20260513T011353.600000000Z"
raw_error = """stderr: codex_api request failed
backend-api websocket disconnected from wss://chatgpt.com/backend-api/codex
tls handshake eof"""

write_json(os.path.join(runs, run_id, "state.json"), {
    "run_id": run_id,
    "change_name": "15-隐藏-status-内部网络错误",
    "sealed": True,
    "status": "failed",
    "stage": "execution",
    "baseline_head": "abc",
    "baseline_diff": "",
    "batch_id": batch_id,
    "sessions": {"codex:executor": "019e1eb7-0aa9-7c01-a91d-0b97e77b85d8"},
    "stages": {},
    "paths": {},
    "workflow_config": workflow,
    "error": raw_error,
})
write_json(os.path.join(batches, batch_id, "state.json"), {
    "batch_id": batch_id,
    "status": "failed",
    "changes": ["15-隐藏-status-内部网络错误", "16-下一个任务"],
    "current_index": 0,
    "run_ids": {"15-隐藏-status-内部网络错误": run_id},
    "failed_change": "15-隐藏-status-内部网络错误",
    "failed_run_id": run_id,
    "error": raw_error,
})
PY

WO="$TMPDIR/wo"
go build -C "$REPO_ROOT" -o "$WO" ./cmd/wo

"$WO" status > status.txt
grep -qF "批量任务 b1 failed 1/2" status.txt
grep -qF "智能体后端连接失败" status.txt
grep -qF -- "- 15-隐藏-status-内部网络错误" status.txt
grep -qF -- "- 16-下一个任务" status.txt
! grep -qF "backend-api" status.txt
! grep -qF "chatgpt.com" status.txt
! grep -qF "tls handshake eof" status.txt
! grep -qF "websocket" status.txt
! grep -qF "stderr" status.txt

"$WO" status --run-id 20260513T011353.638549214Z --json > status.json
python3 - <<'PY'
import json

with open("status.json", encoding="utf-8") as f:
    data = json.load(f)
for field in ["run_id", "change_name", "status", "stage", "stages", "paths", "sessions", "error"]:
    if field not in data:
        raise SystemExit(f"missing field: {field}")
if "backend-api" not in data["error"] or data["status"] != "failed":
    raise SystemExit("raw JSON status contract changed")
PY
! grep -qF "批量任务" status.json

echo "PASS"
