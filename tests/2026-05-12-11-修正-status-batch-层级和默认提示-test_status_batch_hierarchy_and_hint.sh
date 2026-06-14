#!/usr/bin/env bash
set -euo pipefail

# Verifies real wo status output for default batch hints, nested stage details, and JSON stability.

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
    ('20260512T000000.000000001Z', '1-a', 'done', 'done', {'execution': 'completed', 'review_1': 'completed'}, {'codex:executor': 'exec-a', 'codex:reviewer': 'review-a'}),
    ('20260512T000000.000000002Z', '2-b', 'running', 'review_1', {'execution': 'completed'}, {'codex:executor': 'exec-b', 'codex:reviewer': 'review-b'}),
]
for run_id, change, status, stage, stages, sessions in runs_data:
    write_json(os.path.join(runs, run_id, 'state.json'), {
        'run_id': run_id,
        'change_name': change,
        'sealed': True,
        'status': status,
        'stage': stage,
        'baseline_head': 'abc',
        'baseline_diff': '',
        'batch_id': '20260512T000000.000000010Z',
        'sessions': sessions,
        'stages': stages,
        'paths': {},
        'workflow_config': workflow,
        'error': '',
    })

write_json(os.path.join(batches, '20260512T000000.000000010Z', 'state.json'), {
    'batch_id': '20260512T000000.000000010Z',
    'status': 'running',
    'changes': ['1-a', '2-b', '3-c'],
    'current_index': 1,
    'run_ids': {
        '1-a': '20260512T000000.000000001Z',
        '2-b': '20260512T000000.000000002Z',
    },
    'error': '',
})
PY

WO="$TMPDIR/wo"
go build -C "$REPO_ROOT" -o "$WO" ./cmd/wo

"$WO" status > status-default.txt
head -n 1 status-default.txt | grep -qF "批量任务 b1 running 2/3"
! grep -qF "正在查看 repo 最近一次批量工作流，如需查看普通工作流，请使用 wo status -w1" status-default.txt
grep -qF -- "- 1-a" status-default.txt
grep -qF -- "- 2-b" status-default.txt
grep -qF -- "- 3-c" status-default.txt

line_run1=$(grep -nF -- "- 1-a" status-default.txt | cut -d: -f1)
line_run1_stage=$(grep -nE "  执行 exec-a +✓ -" status-default.txt | cut -d: -f1)
line_run2=$(grep -nF -- "- 2-b" status-default.txt | cut -d: -f1)
line_run2_stage=$(grep -nE "  审核 review-b +→ -" status-default.txt | cut -d: -f1)
line_unstarted=$(grep -nF -- "- 3-c" status-default.txt | cut -d: -f1)

test "$line_run1" -lt "$line_run1_stage"
test "$line_run1_stage" -lt "$line_run2"
test "$line_run2" -lt "$line_run2_stage"
test "$line_run2_stage" -lt "$line_unstarted"
tail -n +"$line_unstarted" status-default.txt | grep -qF -- "- 3-c"
! tail -n +"$line_unstarted" status-default.txt | grep -qF "  - 写"
! tail -n +"$line_unstarted" status-default.txt | grep -qF "  - 审"
! tail -n +"$line_unstarted" status-default.txt | grep -qF "  - 存"
! tail -n +"$line_unstarted" status-default.txt | grep -qF "  - 规"
! tail -n +"$line_unstarted" status-default.txt | grep -qF "  执行"
! tail -n +"$line_unstarted" status-default.txt | grep -qF "  审核"

"$WO" status -w1 > status-w1.txt
grep -Eq "执行 exec-b +✓ -" status-w1.txt
! grep -qF "最近一次批量工作流" status-w1.txt
! grep -qF "批量任务" status-w1.txt

"$WO" status --run-id 20260512T000000.000000002Z --json > status.json
grep -qF '"run_id":"20260512T000000.000000002Z"' status.json
grep -qF '"change_name":"2-b"' status.json
grep -qF '"status":"running"' status.json
grep -qF '"stage":"review_1"' status.json
grep -qF '"stages"' status.json
grep -qF '"paths"' status.json
grep -qF '"sessions"' status.json
grep -qF '"error"' status.json
! grep -qF "最近一次批量工作流" status.json
! grep -qF "批量任务" status.json
! grep -qF "工作流" status.json

echo "PASS"
