#!/usr/bin/env bash
# Batch append workflow integration test.
# This script creates a temporary git repository, sets up fake oz and fake agent,
# starts a batch worker, appends changes while the worker is running,
# and verifies serial execution of all changes.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

TMPDIR=$(mktemp -d)
trap 'chmod -R u+w "$TMPDIR" 2>/dev/null || true; rm -rf "$TMPDIR"' EXIT
export HOME="$TMPDIR/home"
mkdir -p "$HOME"

export TMPDIR

cd "$TMPDIR"

# 1. Initialize git repository
git init
git config user.email "test@example.com"
git config user.name "Test"

# 2. Create wo.yaml with zero reviews
cat > wo.yaml <<'EOF'
max_review_iterations: 0
parallel: false
prompts:
  planning: planning
  execution: "{{.Stage}}"
  review: "{{.Stage}}"
  archive: "{{.Stage}} {{.DeliverySummaryPath}}"
EOF

# 3. Create fake oz that reports tasks NOT done (so execution agent runs)
mkdir -p "$TMPDIR/bin"
cat > "$TMPDIR/bin/oz" <<'EOF'
#!/usr/bin/env bash
if [ "$1" = "list" ] && [ "$2" = "--json" ]; then
  echo '{"changes":[{"name":"1-a"},{"name":"2-b"},{"name":"3-c"}]}'
elif [ "$1" = "validate" ]; then
  echo '{"valid":true,"errors":[]}'
elif [ "$1" = "status" ]; then
  change="$2"
  if grep -q '\[x\]' "docs/changes/$change/task.md"; then
    echo '{"tasks":{"total":1,"done":1}}'
  else
    echo '{"tasks":{"total":1,"done":0}}'
  fi
fi
EOF
chmod +x "$TMPDIR/bin/oz"

# 4. Create fake agent inline
cat > "$TMPDIR/bin/legacy-agent" <<'PYEOF'
#!/usr/bin/env python3
import json, os, re, sys, time

tmpdir = os.environ.get('TMPDIR', '/tmp')
agent_log = os.path.join(tmpdir, 'agent.log')
control_started = os.path.join(tmpdir, 'execution-started')
control_continue = os.path.join(tmpdir, 'agent-continue')

prompt = sys.stdin.read()

stage = ""
if "acceptance" in prompt:
    stage = "acceptance"
elif "execution" in prompt:
    stage = "execution"
elif "archive" in prompt:
    stage = "archive"

# Try to get change name from prompt or state dir
change_name = ""
m = re.search(r'docs/changes/([^\s"\']+)', prompt)
if m:
    change_name = m.group(1)
if not change_name:
    for name in ["1-a", "2-b", "3-c"]:
        if name in prompt:
            change_name = name
            break

# Fallback: read most recent run state to get change_name
if not change_name:
    import glob
    state_dir = os.environ.get('STATE_DIR', '')
    if state_dir:
        run_dirs = sorted(glob.glob(os.path.join(state_dir, 'runs', '*')), key=os.path.getmtime, reverse=True)
        for run_dir in run_dirs:
            state_file = os.path.join(run_dir, 'state.json')
            if os.path.exists(state_file):
                try:
                    with open(state_file) as f:
                        data = json.load(f)
                    change_name = data.get('change_name', '')
                    if change_name:
                        break
                except Exception:
                    pass

with open(agent_log, 'a') as f:
    f.write(f"{stage} {change_name}\n")

if stage == "execution":
    open(control_started, 'w').close()
    for _ in range(300):
        if os.path.exists(control_continue):
            break
        time.sleep(0.1)
    if change_name:
        task_path = os.path.join(tmpdir, 'docs', 'changes', change_name, 'task.md')
        os.makedirs(os.path.dirname(task_path), exist_ok=True)
        with open(task_path, 'w') as f:
            f.write("- [x] task\n")

if stage == "acceptance":
    m = re.search(r'(\S*acceptance\.json)', prompt)
    if m:
        path = m.group(1)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, 'w') as f:
            f.write("{\"summary\":\"test acceptance\",\"coverage\":[{\"spec\":\"temporary workflow fixture\",\"tests\":[\"contract-demo\"],\"evidence\":[\"screenshot-demo\"],\"risk\":\"fixture uses fake runtime evidence\"}],\"required_tests\":[{\"id\":\"contract-demo\",\"source\":\"change_contract\",\"path\":\"docs\/changes\/demo\/tests\/demo.acceptance.test.ts\",\"command\":\"true\",\"purpose\":\"cover demo contract\",\"assertions\":[\"batch worker completes appended changes without skipping queue entries and produces screenshot-demo evidence\"]}],\"required_evidence\":[{\"id\":\"screenshot-demo\",\"kind\":\"screenshot\",\"path\":\"test-results\/demo.png\",\"purpose\":\"prove demo runtime\"}]}\n")

if stage == "archive" and change_name:
    archive_dir = os.path.join(tmpdir, 'docs', 'changes', 'archive', f'2026-05-12-{change_name}')
    os.makedirs(archive_dir, exist_ok=True)
    open(os.path.join(archive_dir, 'done'), 'w').close()
    m = re.search(r'(\S*delivery-summary\.md)', prompt)
    if m:
        path = m.group(1)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, 'w') as f:
            f.write("done\n")

print(json.dumps({"type": "thread.started", "thread_id": "fake-thread"}))
print(json.dumps({"type": "turn.completed"}))
PYEOF
chmod +x "$TMPDIR/bin/legacy-agent"
ln -sf "$TMPDIR/bin/legacy-agent" "$TMPDIR/bin/codex"
ln -sf "$TMPDIR/bin/legacy-agent" "$TMPDIR/bin/pi"

# 5. Create active changes
for name in 1-a 2-b 3-c; do
  mkdir -p "docs/changes/$name/tests"
  for f in proposal.md design.md spec.md task.md; do
    echo "$f" > "docs/changes/$name/$f"
  done
  echo "- [ ] task" > "docs/changes/$name/task.md"
  cat > "docs/changes/$name/acceptance.json" <<JSON
{"summary":"test acceptance","coverage":[{"spec":"temporary workflow fixture","tests":["contract-demo"],"evidence":["screenshot-demo"],"risk":"fixture uses fake runtime evidence"}],"required_tests":[{"id":"contract-demo","source":"change_contract","path":"docs/changes/$name/tests/demo.acceptance.test.ts","command":"true","purpose":"cover demo contract","assertions":["batch worker completes appended changes without skipping queue entries and produces screenshot-demo evidence"]}],"required_evidence":[{"id":"screenshot-demo","kind":"screenshot","path":"test-results/demo.png","purpose":"prove demo runtime"}]}
JSON
done

git add .
git commit -m "init"

# 6. Build wo
WO="$TMPDIR/wo"
(cd "$REPO_ROOT" && go build -o "$WO" ./cmd/wo)

export PATH="$TMPDIR/bin:$PATH"
export STATE_DIR=""

# 7. Compute repo key
REPO_ABS=$(cd "$TMPDIR" && pwd)
REPO_KEY=$(python3 -c "
import hashlib, os, re
repo = '$REPO_ABS'
clean = os.path.normpath(os.path.abspath(repo))
name = os.path.basename(clean).lower()
sanitized = re.sub(r'[^a-z0-9]+', '-', name).strip('-')
hash_val = hashlib.sha1(clean.encode()).hexdigest()[:10]
print(f'{sanitized}-{hash_val}')
")
STATE_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/wo/repos/$REPO_KEY"
export STATE_DIR

# 8. Create batch state with one change
mkdir -p "$STATE_DIR/batches/batch-test"
cat > "$STATE_DIR/batches/batch-test/state.json" <<'EOF'
{
  "batch_id": "batch-test",
  "status": "running",
  "changes": ["1-a"],
  "current_index": 0,
  "run_ids": {}
}
EOF

echo "=== Starting batch worker in background ==="
$WO batch --batch-id batch-test --json > "$STATE_DIR/worker.log" 2>&1 &
WORKER_PID=$!

# Wait for execution to start (fake agent creates execution-started file)
for i in {1..100}; do
  if [ -f "$TMPDIR/execution-started" ]; then
    echo "✓ Worker started executing 1-a"
    break
  fi
  sleep 0.1
done

if [ ! -f "$TMPDIR/execution-started" ]; then
  echo "FAIL: Worker did not start executing 1-a"
  kill $WORKER_PID 2>/dev/null || true
  cat "$STATE_DIR/worker.log" || true
  exit 1
fi

# Small delay to ensure worker is in agent's sleep loop
sleep 0.3

echo "=== Appending 2-b and 3-c while worker is running ==="
output=$($WO batch append --batch-id batch-test --change 2-b --change 3-c --json 2>&1)
echo "$output"

if ! echo "$output" | grep -F -q -- "已追加 2 个变更到批量任务 batch-test"; then
  echo "FAIL: Expected append 2 changes"
  kill $WORKER_PID 2>/dev/null || true
  exit 1
fi

# Release agent to continue
echo "=== Releasing agent to continue ==="
touch "$TMPDIR/agent-continue"

# Wait for worker to finish all changes
echo "=== Waiting for worker to complete ==="
for i in {1..120}; do
  if [ -f "$STATE_DIR/batches/batch-test/state.json" ]; then
    STATUS=$(python3 -c "import json; print(json.load(open('$STATE_DIR/batches/batch-test/state.json'))['status'])" 2>/dev/null || echo "")
    if [ "$STATUS" = "done" ]; then
      break
    fi
  fi
  sleep 0.5
done

# Clean up worker
kill $WORKER_PID 2>/dev/null || true
wait $WORKER_PID 2>/dev/null || true

echo "=== Verifying batch state ==="
python3 -m json.tool "$STATE_DIR/batches/batch-test/state.json"

# Verify batch is done with all 3 changes
BATCH_STATUS=$(python3 -c "import json; print(json.load(open('$STATE_DIR/batches/batch-test/state.json'))['status'])" 2>/dev/null || echo "")
if [ "$BATCH_STATUS" != "done" ]; then
  echo "FAIL: Batch should be done, got $BATCH_STATUS"
  exit 1
fi

CURRENT_INDEX=$(python3 -c "import json; print(json.load(open('$STATE_DIR/batches/batch-test/state.json'))['current_index'])" 2>/dev/null || echo "0")
if [ "$CURRENT_INDEX" != "3" ]; then
  echo "FAIL: Batch current_index should be 3, got $CURRENT_INDEX"
  exit 1
fi

# Verify all changes have run_ids
for change in 1-a 2-b 3-c; do
  RUN_ID=$(python3 -c "import json; d=json.load(open('$STATE_DIR/batches/batch-test/state.json')); print(d.get('run_ids', {}).get('$change', ''))" 2>/dev/null || echo "")
  if [ -z "$RUN_ID" ]; then
    echo "FAIL: Missing run_id for $change"
    exit 1
  fi
  echo "✓ $change has run_id $RUN_ID"
done

# Verify execution order from agent log
if [ -f "$TMPDIR/agent.log" ]; then
  echo "=== Agent execution log ==="
  cat "$TMPDIR/agent.log"

  # Build expected order: execution/archive for each change.
  EXPECTED=$(printf "execution 1-a\narchive 1-a\nexecution 2-b\narchive 2-b\nexecution 3-c\narchive 3-c")
  ACTUAL=$(grep -E "^(execution|archive) " "$TMPDIR/agent.log" 2>/dev/null | sed 's/ *$//')

  if [ "$ACTUAL" != "$EXPECTED" ]; then
    echo "FAIL: Execution order mismatch"
    echo "Expected:"
    echo "$EXPECTED"
    echo "Actual:"
    echo "$ACTUAL"
    exit 1
  fi
  echo "✓ Execution order verified: 1-a → 2-b → 3-c"
fi

echo ""
echo "All batch append shell tests passed."
