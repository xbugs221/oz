#!/usr/bin/env bash
set -euo pipefail

# Verifies real wo status usage for short aliases, batch expansion, and JSON stability.

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
repo_key = os.path.basename(repo_path).lower().replace('.', '-') + '-' + hashlib.sha1(repo_path.encode()).hexdigest()[:10]
base = os.path.join(os.environ['XDG_STATE_HOME'], 'wo', 'repos', repo_key)
runs = os.path.join(base, 'runs')
batches = os.path.join(base, 'batches')
os.makedirs(runs, exist_ok=True)
os.makedirs(batches, exist_ok=True)

def write_json(path, data):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, 'w', encoding='utf-8') as f:
        json.dump(data, f, indent=2, ensure_ascii=False)

workflow = {'max_review_iterations': 1, 'stages': {}, 'prompts': {}, 'validation': {'commands': []}}
runs_data = [
    ('20260510T000000.000000001Z', '1-old', 'done', 'done', '20260510T000000.000000010Z', {'execution': 'completed'}, {}),
    ('20260511T000000.000000001Z', '1-a', 'done', 'done', '20260511T000000.000000010Z', {'execution': 'completed', 'review_1': 'completed'}, {'codex:executor': 'exec-a', 'codex:reviewer': 'review-a'}),
    ('20260511T000000.000000002Z', '2-b', 'running', 'review_1', '20260511T000000.000000010Z', {'execution': 'completed'}, {'codex:executor': 'exec-b', 'codex:reviewer': 'review-b'}),
]
for run_id, change, status, stage, batch_id, stages, sessions in runs_data:
    write_json(os.path.join(runs, run_id, 'state.json'), {
        'run_id': run_id,
        'change_name': change,
        'sealed': True,
        'status': status,
        'stage': stage,
        'baseline_head': 'abc',
        'baseline_diff': '',
        'batch_id': batch_id,
        'sessions': sessions,
        'stages': stages,
        'paths': {},
        'workflow_config': workflow,
        'error': '',
    })

write_json(os.path.join(batches, '20260510T000000.000000010Z', 'state.json'), {
    'batch_id': '20260510T000000.000000010Z',
    'status': 'done',
    'changes': ['1-old'],
    'current_index': 1,
    'run_ids': {'1-old': '20260510T000000.000000001Z'},
    'error': '',
})
write_json(os.path.join(batches, '20260511T000000.000000010Z', 'state.json'), {
    'batch_id': '20260511T000000.000000010Z',
    'status': 'running',
    'changes': ['1-a', '2-b', '3-c'],
    'current_index': 1,
    'run_ids': {'1-a': '20260511T000000.000000001Z', '2-b': '20260511T000000.000000002Z'},
    'error': '',
})
PY

WO="$TMPDIR/wo"
go build -C "$REPO_ROOT" -o "$WO" ./cmd/wo

"$WO" status > status-default.txt
grep -qF "→ b1 2/3" status-default.txt
grep -qF -- "- 1-a" status-default.txt
grep -qF -- "- 2-b" status-default.txt
grep -qF -- "- 3-c" status-default.txt
grep -qF "  执行阶段 exec-a ✓ -" status-default.txt
grep -qF "  审核阶段 review-b → -" status-default.txt

line_run1=$(grep -nF -- "- 1-a" status-default.txt | cut -d: -f1)
line_run1_stage=$(grep -nF "  执行阶段 exec-a ✓ -" status-default.txt | cut -d: -f1)
line_run2=$(grep -nF -- "- 2-b" status-default.txt | cut -d: -f1)
line_run2_stage=$(grep -nF "  审核阶段 review-b → -" status-default.txt | cut -d: -f1)
line_unstarted=$(grep -nF -- "- 3-c" status-default.txt | cut -d: -f1)
test "$line_run1" -lt "$line_run1_stage"
test "$line_run1_stage" -lt "$line_run2"
test "$line_run2" -lt "$line_run2_stage"
test "$line_run2_stage" -lt "$line_unstarted"

"$WO" status -b2 > status-b2.txt
grep -qF "→ b2 1/1" status-b2.txt

"$WO" status -w2 > status-w2.txt
grep -qF "执行阶段 exec-a ✓ -" status-w2.txt
! grep -qF "批量任务" status-w2.txt

if "$WO" status -b99 >"$TMPDIR/wo-b99.out" 2>"$TMPDIR/wo-b99.err"; then
    echo "status -b99 should fail" >&2
    exit 1
fi
grep -qF "找不到 b99" "$TMPDIR/wo-b99.err"

"$WO" status --run-id 20260511T000000.000000002Z --json > status.json
grep -qF '"run_id":"20260511T000000.000000002Z"' status.json
grep -qF '"status":"running"' status.json
! grep -qF "批量任务" status.json
! grep -qF "w1" status.json

echo "PASS"
