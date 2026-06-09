#!/usr/bin/env bash
# 验证 wo clean 不会删除 done 或 active-locked run 引用的 Codex/Pi 外部会话记录。
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

WO="$TMPDIR/wo"
go build -C "$REPO_ROOT" -o "$WO" ./cmd/wo

export HOME="$TMPDIR/home"
export XDG_STATE_HOME="$TMPDIR/state"
mkdir -p "$HOME"

REPO="$TMPDIR/repo"
mkdir -p "$REPO"
cd "$REPO"
git init >/dev/null
git config user.email "test@example.com"
git config user.name "Test"
printf 'demo\n' > README.md
git add . && git commit -m "init" >/dev/null

PATHS_JSON="$TMPDIR/paths.json"
LOCK_PID=$$
export LOCK_PID
python3 - "$REPO" "$PATHS_JSON" <<'PY'
import hashlib
import json
import os
import re
import socket
import sqlite3
import sys
from pathlib import Path

def repo_key(repo: Path) -> str:
    clean = str(repo.resolve())
    name = repo.name.lower()
    name = re.sub(r"[^a-z0-9]+", "-", name).strip("-") or "repo"
    return f"{name}-{hashlib.sha1(clean.encode()).hexdigest()[:10]}"

repo = Path(sys.argv[1]).resolve()
paths_json = Path(sys.argv[2])
home = Path(os.environ["HOME"])
state_home = Path(os.environ["XDG_STATE_HOME"])
repo_state = state_home / "wo" / "repos" / repo_key(repo)
base = repo_state / "runs"
batch_base = repo_state / "batches"
done_run = "run-done"
locked_run = "run-locked-failed"
running_run = "run-running-batch-ref"
failed_batch = "batch-failed-running-ref"
done_codex = "019edone-0000-7000-8000-000000000000"
done_pi = "019edone-1111-7000-8000-111111111111"
locked_codex = "019elock-0000-7000-8000-000000000000"
locked_pi = "019elock-1111-7000-8000-111111111111"
running_codex = "019erunb-0000-7000-8000-000000000000"
running_pi = "019erunb-1111-7000-8000-111111111111"

def write_run(run_id: str, status: str, stage: str, sessions: dict[str, str]) -> Path:
    run_dir = base / run_id
    run_dir.mkdir(parents=True, exist_ok=True)
    state = {
        "run_id": run_id,
        "change_name": "25-演示保留agent记录",
        "sealed": True,
        "status": status,
        "stage": stage,
        "error": "",
        "baseline_head": "abc123",
        "baseline_diff": "",
        "sessions": sessions,
        "stages": {},
        "paths": {},
        "workflow_config": {"max_review_iterations": 3, "stages": {}},
    }
    (run_dir / "state.json").write_text(json.dumps(state, indent=2) + "\n", encoding="utf-8")
    return run_dir

def write_batch(batch_id: str, run_id: str) -> Path:
    batch_dir = batch_base / batch_id
    batch_dir.mkdir(parents=True, exist_ok=True)
    state = {
        "batch_id": batch_id,
        "status": "failed",
        "changes": ["1-演示运行中引用"],
        "current_index": 0,
        "run_ids": {"1-演示运行中引用": run_id},
        "failed_change": "1-演示运行中引用",
        "failed_run_id": run_id,
        "error": "demo failure",
    }
    (batch_dir / "state.json").write_text(json.dumps(state, indent=2) + "\n", encoding="utf-8")
    return batch_dir

done_dir = write_run(done_run, "done", "done", {"codex:executor": done_codex, "pi:archiver": done_pi})
locked_dir = write_run(locked_run, "failed", "execution", {"codex:executor": locked_codex, "pi:archiver": locked_pi})
running_dir = write_run(running_run, "running", "execution", {"codex:executor": running_codex, "pi:archiver": running_pi})
failed_batch_dir = write_batch(failed_batch, running_run)
(locked_dir / "lock").write_text(json.dumps({
    "pid": int(os.environ["LOCK_PID"]),
    "hostname": socket.gethostname(),
    "run_id": locked_run,
    "started_at": "2026-05-26T00:00:00Z",
}, indent=2) + "\n", encoding="utf-8")

codex_dir = home / ".codex" / "sessions" / "2026" / "05" / "26"
codex_dir.mkdir(parents=True, exist_ok=True)
done_codex_file = codex_dir / f"rollout-2026-05-26T00-01-00-{done_codex}.jsonl"
locked_codex_file = codex_dir / f"rollout-2026-05-26T00-02-00-{locked_codex}.jsonl"
running_codex_file = codex_dir / f"rollout-2026-05-26T00-03-00-{running_codex}.jsonl"
done_codex_file.write_text('{"type":"thread.started"}\n', encoding="utf-8")
locked_codex_file.write_text('{"type":"thread.started"}\n', encoding="utf-8")
running_codex_file.write_text('{"type":"thread.started"}\n', encoding="utf-8")

pi_dir = home / ".pi" / "agent" / "sessions" / "--tmp-repo--"
pi_dir.mkdir(parents=True, exist_ok=True)
done_pi_file = pi_dir / f"2026-05-26T00-01-00-000Z_{done_pi}.jsonl"
locked_pi_file = pi_dir / f"2026-05-26T00-02-00-000Z_{locked_pi}.jsonl"
running_pi_file = pi_dir / f"2026-05-26T00-03-00-000Z_{running_pi}.jsonl"
done_pi_file.write_text('{"type":"session"}\n', encoding="utf-8")
locked_pi_file.write_text('{"type":"session"}\n', encoding="utf-8")
running_pi_file.write_text('{"type":"session"}\n', encoding="utf-8")

db_path = home / ".pi" / "agent" / "session-index.sqlite"
conn = sqlite3.connect(db_path)
conn.execute("create table sessions(id text primary key, cwd text, jsonl_path text)")
conn.execute("create table messages(id integer primary key, session_id text, body text)")
for session_id, file_path in ((done_pi, done_pi_file), (locked_pi, locked_pi_file), (running_pi, running_pi_file)):
    conn.execute("insert into sessions(id, cwd, jsonl_path) values (?, ?, ?)", (session_id, str(repo), str(file_path)))
    conn.execute("insert into messages(session_id, body) values (?, ?)", (session_id, "keep me"))
conn.commit()
conn.close()

paths_json.write_text(json.dumps({
    "done_dir": str(done_dir),
    "locked_dir": str(locked_dir),
    "running_dir": str(running_dir),
    "failed_batch_dir": str(failed_batch_dir),
    "done_codex_file": str(done_codex_file),
    "locked_codex_file": str(locked_codex_file),
    "running_codex_file": str(running_codex_file),
    "done_pi_file": str(done_pi_file),
    "locked_pi_file": str(locked_pi_file),
    "running_pi_file": str(running_pi_file),
    "db_path": str(db_path),
    "done_pi": done_pi,
    "locked_pi": locked_pi,
    "running_pi": running_pi,
}, indent=2) + "\n", encoding="utf-8")
PY

OUTPUT=$("$WO" clean 2>&1)
printf '%s\n' "$OUTPUT"

python3 - "$PATHS_JSON" <<'PY'
import json
import sqlite3
import sys
from pathlib import Path

paths = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
for key in [
    "done_dir",
    "locked_dir",
    "running_dir",
    "done_codex_file",
    "locked_codex_file",
    "running_codex_file",
    "done_pi_file",
    "locked_pi_file",
    "running_pi_file",
]:
    path = Path(paths[key])
    if not path.exists():
        raise SystemExit(f"{key} was removed unexpectedly: {path}")

failed_batch_dir = Path(paths["failed_batch_dir"])
if failed_batch_dir.exists():
    raise SystemExit(f"failed batch should have been cleaned: {failed_batch_dir}")

conn = sqlite3.connect(paths["db_path"])
for key in ["done_pi", "locked_pi", "running_pi"]:
    sid = paths[key]
    session_rows = conn.execute("select count(*) from sessions where id = ?", (sid,)).fetchone()[0]
    message_rows = conn.execute("select count(*) from messages where session_id = ?", (sid,)).fetchone()[0]
    if session_rows != 1 or message_rows != 1:
        raise SystemExit(f"Pi SQLite rows for protected session {sid} were removed")
conn.close()
PY

if ! printf '%s\n' "$OUTPUT" | grep -q "已跳过"; then
  echo "FAIL: output should mention skipped running task"
  exit 1
fi

echo "PASS: wo clean preserved protected agent session records"
