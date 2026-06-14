#!/usr/bin/env bash
# Purpose: verify a user-selected single change starts as an appendable queue.
# Business flow: choose one active oz change through the real wo CLI, append a
# second change while the first sealed run is executing, then verify serial
# completion through durable queue and run state.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TMPDIR="$(mktemp -d)"

cleanup() {
  touch "$TMPDIR/agent-continue" 2>/dev/null || true
  for _ in $(seq 1 20); do
    if [ -n "${BATCH_STATE:-}" ] && [ -f "$BATCH_STATE" ]; then
      STATUS="$(python3 - "$BATCH_STATE" <<'PY' 2>/dev/null || true
import json
import sys
print(json.load(open(sys.argv[1])).get("status", ""))
PY
)"
      [ "$STATUS" != "running" ] && break
    fi
    sleep 0.1
  done
  for _ in $(seq 1 10); do
    rm -rf "$TMPDIR" 2>/dev/null && return
    sleep 0.1
  done
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

WORK="$TMPDIR/work"
FAKEBIN="$TMPDIR/bin"
HOME_DIR="$TMPDIR/home"
STATE_HOME="$TMPDIR/state"
mkdir -p "$WORK" "$FAKEBIN" "$HOME_DIR" "$STATE_HOME"

cd "$WORK"
git init >/dev/null
git config user.email "test@example.com"
git config user.name "Test User"

cat > wo.yaml <<'YAML'
max_review_iterations: 0
parallel: false
stages:
  planning:
    agent: codex
    reasoning: low
  execution:
    agent: codex
    reasoning: low
  fix:
    agent: codex
    reasoning: low
  review:
    agent: codex
    reasoning: low
  archive:
    agent: codex
    reasoning: low
validation:
  limit: 1
  commands: []
prompts:
  planning: "planning"
  execution: "{{.Stage}} {{.ChangeName}} {{.StatePath}} {{.DeliverySummaryPath}}"
  review: "{{.Stage}} {{.ChangeName}} {{.StatePath}}"
  fix: "{{.Stage}} {{.ChangeName}} {{.StatePath}}"
  archive: "{{.Stage}} {{.ChangeName}} {{.StatePath}} {{.DeliverySummaryPath}}"
YAML

mkdir -p docs/changes/1-a/tests docs/changes/2-b/tests
for change in 1-a 2-b; do
  printf '# %s proposal\n' "$change" > "docs/changes/$change/proposal.md"
  printf '# %s design\n' "$change" > "docs/changes/$change/design.md"
  printf '# %s spec\n' "$change" > "docs/changes/$change/spec.md"
  printf -- '- [ ] implement %s\n' "$change" > "docs/changes/$change/task.md"
  printf '#!/usr/bin/env bash\nexit 0\n' > "docs/changes/$change/tests/smoke.sh"
  cat > "docs/changes/$change/acceptance.json" <<JSON
{"summary":"test acceptance","coverage":[{"spec":"temporary workflow fixture","tests":["contract-demo"],"evidence":["runtime-demo"],"risk":"fixture uses fake runtime evidence"}],"required_tests":[{"id":"contract-demo","source":"change_contract","path":"docs/changes/$change/tests/smoke.sh","command":"bash docs/changes/$change/tests/smoke.sh","purpose":"cover selected change contract","assertions":["single change execution is represented by a one-item batch queue and produces runtime-demo evidence"]}],"required_evidence":[{"id":"runtime-demo","kind":"runtime_log","path":"test-results/demo.log","purpose":"prove queue runtime path"}]}
JSON
done

cat > "$FAKEBIN/oz" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  list)
    printf '{"changes":[{"name":"1-a"},{"name":"2-b"}]}\n'
    ;;
  validate)
    printf '{"valid":true,"errors":[]}\n'
    ;;
  status)
    change="${2:-}"
    if grep -q '\[x\]' "docs/changes/$change/task.md"; then
      printf '{"tasks":{"total":1,"done":1}}\n'
    else
      printf '{"tasks":{"total":1,"done":0}}\n'
    fi
    ;;
  *)
    printf 'unexpected oz command\n' >&2
    exit 1
    ;;
esac
SH
chmod +x "$FAKEBIN/oz"

cat > "$FAKEBIN/codex" <<'PY'
#!/usr/bin/env python3
"""Fake Codex CLI for the single-change queue acceptance test."""
import json
import os
import re
import sys
import time

tmpdir = os.environ["WO_TEST_TMPDIR"]
prompt = sys.stdin.read()
parts = prompt.strip().split()
stage = parts[0] if len(parts) > 0 else ""
change = parts[1] if len(parts) > 1 else ""
state_path = parts[2] if len(parts) > 2 else ""
delivery_path = parts[3] if len(parts) > 3 else ""

with open(os.path.join(tmpdir, "agent.log"), "a", encoding="utf-8") as log:
    log.write(f"{stage} {change}\n")

if stage == "acceptance":
    acceptance_path = parts[3] if len(parts) > 3 else ""
    if acceptance_path:
        os.makedirs(os.path.dirname(acceptance_path), exist_ok=True)
        body = {
            "summary": f"acceptance for {change}",
            "required_tests": [{
                "id": "contract-demo",
                "source": "change_contract",
                "path": f"docs/changes/{change}/tests/demo.acceptance.test.ts",
                "command": "true",
                "purpose": "cover the selected change contract",
                "assertions": ["single change execution is represented by a one-item batch queue"],
            }],
            "required_evidence": [{
                "id": "screenshot-demo",
                "kind": "screenshot",
                "path": "test-results/demo.png",
                "purpose": "prove the queue runtime path",
            }],
        }
        with open(acceptance_path, "w", encoding="utf-8") as acceptance:
            json.dump(body, acceptance)
            acceptance.write("\n")

if stage == "execution":
    open(os.path.join(tmpdir, "execution-started"), "a", encoding="utf-8").close()
    for _ in range(200):
        if os.path.exists(os.path.join(tmpdir, "agent-continue")):
            break
        time.sleep(0.1)
    task_path = os.path.join(os.getcwd(), "docs", "changes", change, "task.md")
    with open(task_path, "w", encoding="utf-8") as task:
        task.write(f"- [x] implement {change}\n")

if stage == "archive":
    if delivery_path:
        os.makedirs(os.path.dirname(delivery_path), exist_ok=True)
        with open(delivery_path, "w", encoding="utf-8") as delivery:
            delivery.write(f"delivered {change}\n")
    archive_dir = os.path.join(os.getcwd(), "docs", "changes", "archive", f"2026-05-21-{change}")
    os.makedirs(archive_dir, exist_ok=True)
    source_dir = os.path.join(os.getcwd(), "docs", "changes", change)
    for name in ("proposal.md", "design.md", "spec.md", "task.md"):
        src = os.path.join(source_dir, name)
        dst = os.path.join(archive_dir, name)
        if os.path.exists(src):
            with open(src, "r", encoding="utf-8") as inf, open(dst, "w", encoding="utf-8") as outf:
                outf.write(inf.read())

session = re.sub(r"[^a-z0-9-]+", "-", f"{stage}-{change}".lower()).strip("-")
print(json.dumps({"type": "thread.started", "thread_id": session}))
print(json.dumps({"type": "turn.completed"}))
PY
chmod +x "$FAKEBIN/codex"
ln -sf "$FAKEBIN/codex" "$FAKEBIN/pi"
ln -sf "$FAKEBIN/codex" "$FAKEBIN/agy"

git add .
git commit -m "init" >/dev/null

WO="$TMPDIR/wo"
(cd "$REPO_ROOT" && go build -o "$WO" ./cmd/wo)

export PATH="$FAKEBIN:/usr/bin:/bin"
export HOME="$HOME_DIR"
export XDG_STATE_HOME="$STATE_HOME"
export WO_TEST_TMPDIR="$TMPDIR"

printf '2\n1\n' | "$WO" > "$TMPDIR/wo-start.out" 2> "$TMPDIR/wo-start.err"

BATCH_STATE=""
for _ in $(seq 1 40); do
  BATCH_STATE="$(python3 - <<'PY'
import glob
import os
paths = glob.glob(os.path.join(os.environ["XDG_STATE_HOME"], "wo", "repos", "*", "batches", "*", "state.json"))
print(paths[0] if paths else "")
PY
)"
  [ -n "$BATCH_STATE" ] && break
  sleep 0.1
done

if [ -z "$BATCH_STATE" ]; then
  echo "FAIL: selecting one change must create an appendable queue under batches/" >&2
  echo "--- stdout ---" >&2
  cat "$TMPDIR/wo-start.out" >&2 || true
  echo "--- stderr ---" >&2
  cat "$TMPDIR/wo-start.err" >&2 || true
  exit 1
fi

BATCH_ID="$(basename "$(dirname "$BATCH_STATE")")"

python3 - "$BATCH_STATE" <<'PY'
import json
import sys
state = json.load(open(sys.argv[1]))
assert state["status"] == "running", state
assert state["changes"] == ["1-a"], state
assert state["current_index"] == 0, state
PY

for _ in $(seq 1 100); do
  [ -f "$TMPDIR/execution-started" ] && break
  sleep 0.1
done
if [ ! -f "$TMPDIR/execution-started" ]; then
  echo "FAIL: first queued run did not enter execution" >&2
  exit 1
fi

FIRST_RUN="$(python3 - "$BATCH_STATE" <<'PY'
import json
import sys
state = json.load(open(sys.argv[1]))
print(state.get("run_ids", {}).get("1-a", ""))
PY
)"
if [ -z "$FIRST_RUN" ]; then
  echo "FAIL: queue did not record run id for first change" >&2
  exit 1
fi

"$WO" batch append --batch-id "$BATCH_ID" --change 2-b --json > "$TMPDIR/append.out"

python3 - "$BATCH_STATE" "$FIRST_RUN" <<'PY'
import json
import sys
state = json.load(open(sys.argv[1]))
assert state["changes"] == ["1-a", "2-b"], state
assert state["current_index"] == 0, state
assert state["run_ids"]["1-a"] == sys.argv[2], state
PY

touch "$TMPDIR/agent-continue"

for _ in $(seq 1 160); do
  STATUS="$(python3 - "$BATCH_STATE" <<'PY'
import json
import sys
print(json.load(open(sys.argv[1])).get("status", ""))
PY
)"
  [ "$STATUS" = "done" ] && break
  sleep 0.25
done

python3 - "$BATCH_STATE" <<'PY'
import json
import sys
state = json.load(open(sys.argv[1]))
assert state["status"] == "done", state
assert state["current_index"] == 2, state
assert state["changes"] == ["1-a", "2-b"], state
assert state["run_ids"].get("1-a"), state
assert state["run_ids"].get("2-b"), state
assert state["run_ids"]["1-a"] != state["run_ids"]["2-b"], state
PY

EXPECTED="$(printf 'execution 1-a\narchive 1-a\nexecution 2-b\narchive 2-b\n')"
ACTUAL="$(grep -E '^(execution|archive) ' "$TMPDIR/agent.log" || true)"
if [ "$ACTUAL" != "$EXPECTED" ]; then
  echo "FAIL: changes did not execute serially through one queue" >&2
  echo "Expected:" >&2
  printf '%s\n' "$EXPECTED" >&2
  echo "Actual:" >&2
  printf '%s\n' "$ACTUAL" >&2
  exit 1
fi

echo "PASS: single selected change is an appendable serial queue"
