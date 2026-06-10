#!/usr/bin/env bash
# Verifies wo status shows human duration summaries for completed workflow stages.
set -euo pipefail

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
repo_name = os.path.basename(repo_path).lower().replace(".", "-")
repo_key = repo_name + "-" + hashlib.sha1(repo_path.encode()).hexdigest()[:10]
base = os.path.join(os.environ["XDG_STATE_HOME"], "wo", "repos", repo_key)
runs = os.path.join(base, "runs")
batches = os.path.join(base, "batches")
os.makedirs(runs, exist_ok=True)
os.makedirs(batches, exist_ok=True)

def write_json(path, data):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False)

workflow = {
    "max_review_iterations": 1,
    "stages": {
        "execution": {"tool": "codex", "reasoning": "low", "fast": False},
        "review_1": {"tool": "codex", "reasoning": "high", "fast": False},
        "fix_1": {"tool": "codex", "reasoning": "low", "fast": False},
        "archive": {"tool": "codex", "reasoning": "low", "fast": False},
    },
    "validation": {"max_attempts_per_stage": 3, "commands": []},
}

workflow_multi_round = {
    "max_review_iterations": 2,
    "stages": {
        "execution": {"tool": "codex", "reasoning": "low", "fast": False},
        "review_1": {"tool": "codex", "reasoning": "high", "fast": False},
        "fix_1": {"tool": "codex", "reasoning": "low", "fast": False},
        "review_2": {"tool": "codex", "reasoning": "high", "fast": False},
        "fix_2": {"tool": "codex", "reasoning": "low", "fast": False},
        "archive": {"tool": "codex", "reasoning": "low", "fast": False},
    },
    "validation": {"max_attempts_per_stage": 3, "commands": []},
}

main_run = "20260525T040000.000000000Z"
write_json(os.path.join(runs, main_run, "state.json"), {
    "run_id": main_run,
    "change_name": "1-演示统计",
    "sealed": True,
    "status": "done",
    "stage": "done",
    "baseline_head": "abc",
    "baseline_diff": "",
    "batch_id": "20260525T045000.000000000Z",
    "sessions": {
        "codex:executor": "executor-thread",
        "codex:reviewer": "reviewer-thread",
        "codex:archiver": "archiver-thread",
    },
    "stages": {
        "execution": "completed",
        "review_1": "completed",
        "archive": "completed",
    },
    "stage_timings": {
        "execution": {
            "started_at": "2026-05-25T00:00:00Z",
            "finished_at": "2026-05-25T01:40:00Z"
        },
        "review_1": {
            "started_at": "2026-05-25T01:40:00Z",
            "finished_at": "2026-05-25T03:40:00Z"
        },
        "archive": {
            "started_at": "2026-05-25T03:40:00Z",
            "finished_at": "2026-05-25T05:00:00Z"
        },
    },
    "paths": {},
    "workflow_config": workflow,
    "error": "",
})

skip_run = "20260525T030000.000000000Z"
write_json(os.path.join(runs, skip_run, "state.json"), {
    "run_id": skip_run,
    "change_name": "2-跳过阶段",
    "sealed": True,
    "status": "done",
    "stage": "done",
    "baseline_head": "abc",
    "baseline_diff": "",
    "sessions": {
        "codex:executor": "executor-thread",
        "codex:reviewer": "reviewer-thread",
        "codex:archiver": "archiver-thread",
    },
    "stages": {
        "execution": "completed",
        "review_1": "completed",
        "archive": "completed",
    },
    "stage_timings": {
        "execution": {
            "started_at": "2026-05-25T00:00:00Z",
            "finished_at": "2026-05-25T00:01:30Z"
        },
        "archive": {
            "started_at": "2026-05-25T00:01:30Z",
            "finished_at": "2026-05-25T00:02:45Z"
        },
    },
    "paths": {},
    "workflow_config": workflow,
    "error": "",
})

multi_run = "20260525T020000.000000000Z"
write_json(os.path.join(runs, multi_run, "state.json"), {
    "run_id": multi_run,
    "change_name": "4-多轮修审",
    "sealed": True,
    "status": "done",
    "stage": "done",
    "baseline_head": "abc",
    "baseline_diff": "",
    "sessions": {
        "codex:executor": "executor-thread",
        "codex:reviewer": "reviewer-thread",
        "codex:fixer": "fixer-thread",
        "codex:archiver": "archiver-thread",
    },
    "stages": {
        "execution": "completed",
        "review_1": "completed",
        "fix_1": "completed",
        "review_2": "completed",
        "fix_2": "completed",
        "archive": "completed",
    },
    "stage_timings": {
        "execution": {
            "started_at": "2026-05-25T00:00:00Z",
            "finished_at": "2026-05-25T00:01:00Z"
        },
        "review_1": {
            "started_at": "2026-05-25T00:01:00Z",
            "finished_at": "2026-05-25T00:03:00Z"
        },
        "fix_1": {
            "started_at": "2026-05-25T00:03:00Z",
            "finished_at": "2026-05-25T00:06:00Z"
        },
        "review_2": {
            "started_at": "2026-05-25T00:06:00Z",
            "finished_at": "2026-05-25T00:10:00Z"
        },
        "fix_2": {
            "started_at": "2026-05-25T00:10:00Z",
            "finished_at": "2026-05-25T00:15:00Z"
        },
        "archive": {
            "started_at": "2026-05-25T00:15:00Z",
            "finished_at": "2026-05-25T00:21:00Z"
        },
    },
    "paths": {},
    "workflow_config": workflow_multi_round,
    "error": "",
})

write_json(os.path.join(batches, "20260525T045000.000000000Z", "state.json"), {
    "batch_id": "20260525T045000.000000000Z",
    "status": "running",
    "changes": ["1-演示统计", "3-尚未开始"],
    "current_index": 0,
    "run_ids": {"1-演示统计": main_run},
    "error": "",
})
PY

WO="$TMPDIR/wo"
go build -C "$REPO_ROOT" -o "$WO" ./cmd/wo

"$WO" status -w1 > status-w1.txt
grep -qF -- "执行阶段 executor-thread ✓ 100.00" status-w1.txt
grep -qF -- "审核阶段 reviewer-thread ✓ 120.00" status-w1.txt
grep -qF -- "归档阶段 archiver-thread ✓ 80.00" status-w1.txt
! grep -qF -- "耗时" status-w1.txt
! grep -qF -- "分钟" status-w1.txt

python3 - <<'PY'
from pathlib import Path

lines = Path("status-w1.txt").read_text(encoding="utf-8").splitlines()
execution = next(i for i, line in enumerate(lines) if "执行阶段 executor-thread ✓ 100.00" in line)
review = next(i for i, line in enumerate(lines) if "审核阶段 reviewer-thread ✓ 120.00" in line)
archive = next(i for i, line in enumerate(lines) if "归档阶段 archiver-thread ✓ 80.00" in line)
if not execution < review < archive:
    raise SystemExit("duration columns must stay in stage order")
PY

"$WO" status -w2 > status-w2.txt
grep -qF -- "执行阶段 executor-thread ✓ 1.50" status-w2.txt
grep -qF -- "归档阶段 archiver-thread ✓ 1.25" status-w2.txt
grep -qF -- "审核阶段 reviewer-thread ✓ -" status-w2.txt

"$WO" status -w3 > status-w3.txt
grep -qF -- "审核阶段 reviewer-thread ✓✓ 6.00" status-w3.txt
grep -qF -- "修正阶段 fixer-thread ✓✓ 8.00" status-w3.txt
! grep -qF -- "review_1" status-w3.txt
! grep -qF -- "review_2" status-w3.txt
! grep -qF -- "fix_1" status-w3.txt
! grep -qF -- "fix_2" status-w3.txt

"$WO" status > status-batch.txt
head -1 status-batch.txt | grep -qF -- "- 1-演示统计"
! grep -qF "→ b1 1/2" status-batch.txt
grep -qF -- "- 1-演示统计" status-batch.txt
grep -qF -- "  执行阶段 executor-thread ✓ 100.00" status-batch.txt
grep -qF -- "  审核阶段 reviewer-thread ✓ 120.00" status-batch.txt
grep -qF -- "- 3-尚未开始" status-batch.txt

python3 - <<'PY'
from pathlib import Path

lines = Path("status-batch.txt").read_text(encoding="utf-8").splitlines()
unstarted = next(i for i, line in enumerate(lines) if line == "- 3-尚未开始")
if any("阶段" in line for line in lines[unstarted + 1:]):
    raise SystemExit("unstarted change must not have stage lines")
PY

"$WO" status --run-id 20260525T040000.000000000Z --json > status.json
python3 - <<'PY'
import json

with open("status.json", encoding="utf-8") as f:
    data = json.load(f)

for field in ["run_id", "change_name", "status", "stage", "stages", "paths", "sessions", "error"]:
    if field not in data:
        raise SystemExit(f"missing JSON field: {field}")

text = json.dumps(data, ensure_ascii=False)
for banned in ["stage_timings", "耗时", "分钟"]:
    if banned in text:
        raise SystemExit(f"JSON status leaked {banned}")
PY

echo "PASS"
