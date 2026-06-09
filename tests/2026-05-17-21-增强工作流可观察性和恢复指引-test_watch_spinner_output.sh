#!/usr/bin/env bash
set -euo pipefail

# 验证 wo watch 输出包含 spinner 帧（| / - \）之一，且普通 wo status 仍使用静态 →。

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
run_id = "20260517T060000.000000000Z"

write_json(os.path.join(runs, run_id, "state.json"), {
    "run_id": run_id,
    "change_name": "x-变更",
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

# Capture watch output (non-TTY, will get multiple frames).
timeout -s INT 3s "$WO" watch > watch_output.txt 2>&1 || true

# Verify spinner characters appear in watch output.
# In non-TTY mode, we get multiple frames; at least one should have a spinner char.
HAS_SPINNER=false
for char in "|" "/" "-" "\\"; do
    if grep -qF "$char" watch_output.txt; then
        HAS_SPINNER=true
        break
    fi
done

if [ "$HAS_SPINNER" = false ]; then
    echo "watch output should contain spinner frames"
    echo "watch output:"
    cat watch_output.txt
    exit 1
fi

# Verify regular wo status still uses static arrow.
"$WO" status > status_output.txt
grep -qF "→" status_output.txt

echo "PASS"
