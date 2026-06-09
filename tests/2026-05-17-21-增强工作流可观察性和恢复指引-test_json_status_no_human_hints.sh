#!/usr/bin/env bash
set -euo pipefail

# 验证 --json 状态的机器接口不变：不包含人类提示、restart 命令、rm -rf 或 spinner。

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
os.makedirs(runs, exist_ok=True)

def write_json(path, data):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False)

workflow = {"max_review_iterations": 1, "stages": {}, "prompts": {}, "validation": {"commands": []}}
run_id = "20260517T070000.000000000Z"

write_json(os.path.join(runs, run_id, "state.json"), {
    "run_id": run_id,
    "change_name": "1-演示变更",
    "sealed": True,
    "status": "failed",
    "stage": "execution",
    "baseline_head": "abc",
    "baseline_diff": "",
    "sessions": {"codex:executor": "thread-1"},
    "stages": {"execution": "interrupted"},
    "paths": {},
    "workflow_config": workflow,
    "error": "backend-api connection failed",
})
PY

WO="$TMPDIR/wo"
go build -C "$REPO_ROOT" -o "$WO" ./cmd/wo

"$WO" status --run-id 20260517T070000.000000000Z --json > status.json

# Verify it's valid JSON.
python3 - <<'PY'
import json

with open("status.json", encoding="utf-8") as f:
    data = json.load(f)

# Required fields must exist.
for field in ["run_id", "change_name", "status", "stage", "stages", "paths", "sessions", "error"]:
    if field not in data:
        raise SystemExit(f"missing required field: {field}")

# Must not contain human hints.
text = json.dumps(data, ensure_ascii=False)
for banned in ["wo restart", "rm -rf", "清理:"]:
    if banned in text:
        raise SystemExit(f"json should not contain {banned}")

print("JSON contract preserved")
PY

echo "PASS"
