#!/usr/bin/env bash
# 验证 wo clean 清理失败 run 时，同步删除它引用的 Codex/Pi JSONL 和 Pi SQLite 会话记录。
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
python3 - "$REPO" "$PATHS_JSON" <<'PY'
import hashlib
import json
import os
import re
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
run_id = "run-failed-agent-records"
codex_id = "019eaaaa-bbbb-7ccc-8ddd-eeeeeeeeeeee"
pi_id = "019effff-1111-7222-8333-444444444444"
unrelated_codex_id = "019e0000-0000-7000-8000-000000000000"
unrelated_pi_id = "019e9999-9999-7999-8999-999999999999"

run_dir = state_home / "wo" / "repos" / repo_key(repo) / "runs" / run_id
run_dir.mkdir(parents=True, exist_ok=True)
state = {
    "run_id": run_id,
    "change_name": "25-演示清理agent记录",
    "sealed": True,
    "status": "failed",
    "stage": "execution",
    "error": "agent failed",
    "baseline_head": "abc123",
    "baseline_diff": "",
    "sessions": {
        "codex:executor": codex_id,
        "pi:archiver": pi_id,
    },
    "stages": {},
    "paths": {},
    "workflow_config": {"max_review_iterations": 3, "stages": {}},
}
(run_dir / "state.json").write_text(json.dumps(state, indent=2) + "\n", encoding="utf-8")

codex_dir = home / ".codex" / "sessions" / "2026" / "05" / "26"
codex_dir.mkdir(parents=True, exist_ok=True)
codex_file = codex_dir / f"rollout-2026-05-26T00-01-00-{codex_id}.jsonl"
codex_file.write_text('{"type":"thread.started"}\n', encoding="utf-8")
unrelated_codex_file = codex_dir / f"rollout-2026-05-26T00-02-00-{unrelated_codex_id}.jsonl"
unrelated_codex_file.write_text('{"type":"thread.started"}\n', encoding="utf-8")

pi_dir = home / ".pi" / "agent" / "sessions" / "--tmp-repo--"
pi_dir.mkdir(parents=True, exist_ok=True)
pi_file = pi_dir / f"2026-05-26T00-01-00-000Z_{pi_id}.jsonl"
pi_file.write_text('{"type":"session"}\n', encoding="utf-8")
unrelated_pi_file = pi_dir / f"2026-05-26T00-02-00-000Z_{unrelated_pi_id}.jsonl"
unrelated_pi_file.write_text('{"type":"session"}\n', encoding="utf-8")

db_path = home / ".pi" / "agent" / "session-index.sqlite"
conn = sqlite3.connect(db_path)
conn.execute("create table sessions(id text primary key, cwd text, jsonl_path text)")
conn.execute("create table messages(id integer primary key, session_id text, body text)")
conn.execute("insert into sessions(id, cwd, jsonl_path) values (?, ?, ?)", (pi_id, str(repo), str(pi_file)))
conn.execute("insert into sessions(id, cwd, jsonl_path) values (?, ?, ?)", (unrelated_pi_id, str(repo), str(unrelated_pi_file)))
conn.execute("insert into messages(session_id, body) values (?, ?)", (pi_id, "clean me"))
conn.execute("insert into messages(session_id, body) values (?, ?)", (unrelated_pi_id, "keep me"))
conn.commit()
conn.close()

paths_json.write_text(json.dumps({
    "run_dir": str(run_dir),
    "codex_file": str(codex_file),
    "unrelated_codex_file": str(unrelated_codex_file),
    "pi_file": str(pi_file),
    "unrelated_pi_file": str(unrelated_pi_file),
    "db_path": str(db_path),
    "pi_id": pi_id,
    "unrelated_pi_id": unrelated_pi_id,
}, indent=2) + "\n", encoding="utf-8")
PY

OUTPUT=$("$WO" clean --agent-sessions 2>&1)
printf '%s\n' "$OUTPUT"

export CLEAN_OUTPUT="$OUTPUT"
python3 - "$PATHS_JSON" <<'PY'
import json
import os
import sqlite3
import sys
from pathlib import Path

paths = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))

def must_not_exist(key: str) -> None:
    path = Path(paths[key])
    if path.exists():
        raise SystemExit(f"{key} still exists: {path}")

def must_exist(key: str) -> None:
    path = Path(paths[key])
    if not path.exists():
        raise SystemExit(f"{key} was removed unexpectedly: {path}")

must_not_exist("run_dir")
must_not_exist("codex_file")
must_not_exist("pi_file")
must_exist("unrelated_codex_file")
must_exist("unrelated_pi_file")

conn = sqlite3.connect(paths["db_path"])
cleaned_sessions = conn.execute("select count(*) from sessions where id = ?", (paths["pi_id"],)).fetchone()[0]
cleaned_messages = conn.execute("select count(*) from messages where session_id = ?", (paths["pi_id"],)).fetchone()[0]
kept_sessions = conn.execute("select count(*) from sessions where id = ?", (paths["unrelated_pi_id"],)).fetchone()[0]
kept_messages = conn.execute("select count(*) from messages where session_id = ?", (paths["unrelated_pi_id"],)).fetchone()[0]
conn.close()
if cleaned_sessions or cleaned_messages:
    raise SystemExit("Pi SQLite rows for cleaned session still exist")
if kept_sessions != 1 or kept_messages != 1:
    raise SystemExit("unrelated Pi SQLite rows were not preserved")

output = os.environ["CLEAN_OUTPUT"]
if "agent" not in output or "会话" not in output:
    raise SystemExit("clean output does not mention agent session record cleanup")
if "不回滚代码改动" not in output:
    raise SystemExit("clean output lost rollback safety hint")
PY

echo "PASS: wo clean removed failed run agent session records"
