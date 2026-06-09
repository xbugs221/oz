#!/usr/bin/env bash
set -euo pipefail

# Test wo status batch human output with real file layout.

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

export XDG_STATE_HOME="$TMPDIR/state"
REPO="$TMPDIR/repo"
mkdir -p "$REPO"
cd "$REPO"
git init
python3 -c "
import json, os

repo_key = 'repo-' + os.popen('git rev-parse --show-toplevel').read().strip().split('/')[-1][:4]
import hashlib
repo_path = os.popen('git rev-parse --show-toplevel').read().strip()
repo_key = os.path.basename(repo_path).lower().replace('.', '-') + '-' + hashlib.sha1(repo_path.encode()).hexdigest()[:10]

base = os.path.join('$XDG_STATE_HOME', 'wo', 'repos', repo_key)
runs = os.path.join(base, 'runs')
batches = os.path.join(base, 'batches')
os.makedirs(runs, exist_ok=True)
os.makedirs(batches, exist_ok=True)

# run-1 done
run1 = {
    'run_id': 'run-1',
    'change_name': '1-a',
    'sealed': True,
    'status': 'done',
    'stage': 'done',
    'baseline_head': 'abc',
    'baseline_diff': '',
    'batch_id': 'batch-1',
    'batch_index': 1,
    'batch_total': 3,
    'sessions': {},
    'stages': {'execution': 'completed', 'archive': 'completed'},
    'paths': {},
    'workflow_config': {'stages': {}, 'prompts': {}, 'validation': {'commands': []}}
}
os.makedirs(os.path.join(runs, 'run-1'), exist_ok=True)
with open(os.path.join(runs, 'run-1', 'state.json'), 'w') as f:
    json.dump(run1, f, indent=2)

# run-2 running
run2 = {
    'run_id': 'run-2',
    'change_name': '2-b',
    'sealed': True,
    'status': 'running',
    'stage': 'review_1',
    'baseline_head': 'abc',
    'baseline_diff': '',
    'batch_id': 'batch-1',
    'batch_index': 2,
    'batch_total': 3,
    'sessions': {'codex:executor': 'exec-thread'},
    'stages': {'execution': 'completed'},
    'paths': {},
    'workflow_config': {'stages': {}, 'prompts': {}, 'validation': {'commands': []}}
}
os.makedirs(os.path.join(runs, 'run-2'), exist_ok=True)
with open(os.path.join(runs, 'run-2', 'state.json'), 'w') as f:
    json.dump(run2, f, indent=2)

# batch state
batch = {
    'batch_id': 'batch-1',
    'status': 'running',
    'changes': ['1-a', '2-b', '3-c'],
    'current_index': 1,
    'run_ids': {'1-a': 'run-1', '2-b': 'run-2'},
    'error': ''
}
os.makedirs(os.path.join(batches, 'batch-1'), exist_ok=True)
with open(os.path.join(batches, 'batch-1', 'state.json'), 'w') as f:
    json.dump(batch, f, indent=2)
"

# Build wo binary from source tree
WO="$TMPDIR/wo"
go build -C "$REPO_ROOT" -o "$WO" ./cmd/wo

OUTPUT=$("$WO" status 2>/dev/null || true)

echo "=== wo status output ==="
echo "$OUTPUT"

# Assertions
for want in "→ b1 2/3" "- 1-a" "- 2-b" "- 3-c"; do
    if ! echo "$OUTPUT" | grep -qF -- "$want"; then
        echo "FAIL: missing '$want'"
        exit 1
    fi
done

# Check every created run stage detail is indented
if ! echo "$OUTPUT" | grep -q "  执行阶段 - ✓ -"; then
    echo "FAIL: missing completed run stage detail"
    exit 1
fi
if ! echo "$OUTPUT" | grep -q "  执行阶段 exec-thread ✓ -"; then
    echo "FAIL: missing indented stage detail"
    exit 1
fi

# JSON contract check
JSON_OUT=$("$WO" status --run-id run-2 --json 2>/dev/null || true)
echo "=== wo status --json output ==="
echo "$JSON_OUT"

for want in '"run_id"' '"change_name"' '"status"' '"stage"' '"stages"' '"paths"' '"sessions"' '"error"'; do
    if ! echo "$JSON_OUT" | grep -q "$want"; then
        echo "FAIL: JSON missing $want"
        exit 1
    fi
done

if echo "$JSON_OUT" | grep -q "批量任务"; then
    echo "FAIL: JSON should not contain batch human terms"
    exit 1
fi

echo "PASS"
