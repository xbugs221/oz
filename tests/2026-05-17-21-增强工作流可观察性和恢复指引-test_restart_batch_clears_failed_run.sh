#!/usr/bin/env bash
set -euo pipefail

# 验证 wo restart -bN 删除失败 run 关联后，batch worker 可为当前 change 创建新 run。

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
TMPDIR=$(mktemp -d)
trap 'chmod -R u+w "$TMPDIR" 2>/dev/null || true; rm -rf "$TMPDIR" 2>/dev/null || true' EXIT

export XDG_STATE_HOME="$TMPDIR/state"
REPO="$TMPDIR/repo"
FAKEBIN="$TMPDIR/fakebin"
mkdir -p "$REPO"
mkdir -p "$FAKEBIN"
cd "$REPO"
git init >/dev/null
git config user.email test@example.com
git config user.name Test

# Create actual change directory so batch worker can create runs.
mkdir -p docs/changes/1-重启变更
cat > docs/changes/1-重启变更/brief.md <<'EOF'
# 重启变更
EOF
cat > docs/changes/1-重启变更/proposal.md <<'EOF'
# 重启变更
EOF
cat > docs/changes/1-重启变更/design.md <<'EOF'
# 设计
EOF
cat > docs/changes/1-重启变更/spec.md <<'EOF'
# 重启变更规格

## 需求

### 需求：重启后系统必须创建新运行

系统必须在重启 batch 后为当前 change 创建新的运行记录。

#### 场景：重启后创建新运行

- **给定** batch 状态为 failed
- **当** 用户运行 wo restart -b1
- **则** 系统必须为当前 change 创建新的 run
- **且** 新 run 不得复用旧 run_id
EOF
cat > docs/changes/1-重启变更/task.md <<'EOF'
- [ ] task
EOF
cat > docs/changes/1-重启变更/acceptance.json <<'JSON'
{"summary":"test acceptance","coverage":[{"spec":"temporary workflow fixture","tests":["contract-demo"],"evidence":["runtime-demo"],"risk":"fixture uses fake runtime evidence"}],"required_tests":[{"id":"contract-demo","source":"change_contract","path":"docs/changes/1-重启变更/tests/restart_batch_creates_new_run_test.sh","command":"bash docs/changes/1-重启变更/tests/restart_batch_creates_new_run_test.sh","purpose":"cover restart contract","assertions":["restart clears failed batch run association, creates a new run, and produces runtime-demo evidence"]}],"required_evidence":[{"id":"runtime-demo","kind":"runtime_log","path":"test-results/restart.log","purpose":"prove restart runtime path"}]}
JSON
mkdir -p docs/changes/1-重启变更/tests
cat > docs/changes/1-重启变更/tests/restart_batch_creates_new_run_test.sh <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
test -s docs/changes/1-重启变更/task.md
EOF

# Initial commit is needed for gitSnapshot in createRun.
git add docs/ >/dev/null
git commit --allow-empty -m init >/dev/null

python3 - <<'PY'
import hashlib
import json
import os

repo_path = os.getcwd()
repo_key = os.path.basename(repo_path).lower().replace(".", "-") + "-" + hashlib.sha1(repo_path.encode()).hexdigest()[:10]
base = os.path.join(os.environ["XDG_STATE_HOME"], "wo", "repos", repo_key)
runs = os.path.join(base, "runs")
batches = os.path.join(base, "batches")
os.makedirs(runs, exist_ok=True)
os.makedirs(batches, exist_ok=True)

def write_json(path, data):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False)

workflow = {"max_review_iterations": 1, "stages": {}, "prompts": {}, "validation": {"commands": []}}
RUN_ID = "20260517T030000.000000000Z"
BATCH_ID = "20260517T030000.500000000Z"

write_json(os.path.join(runs, RUN_ID, "state.json"), {
    "run_id": RUN_ID,
    "change_name": "1-重启变更",
    "sealed": True,
    "status": "failed",
    "stage": "execution",
    "baseline_head": "abc",
    "baseline_diff": "",
    "batch_id": BATCH_ID,
    "sessions": {},
    "stages": {},
    "paths": {},
    "workflow_config": workflow,
    "error": "intentional test failure",
})
write_json(os.path.join(batches, BATCH_ID, "state.json"), {
    "batch_id": BATCH_ID,
    "status": "failed",
    "changes": ["1-重启变更"],
    "current_index": 0,
    "run_ids": {"1-重启变更": RUN_ID},
    "failed_change": "1-重启变更",
    "failed_run_id": RUN_ID,
    "error": "failed",
})
PY

WO="$TMPDIR/wo"
go build -C "$REPO_ROOT" -o "$WO" ./cmd/wo
cat > "$FAKEBIN/codex" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '{"type":"thread.started","thread_id":"fake-thread"}\n'
exit 7
EOF
chmod +x "$FAKEBIN/codex"
ln -sf "$FAKEBIN/codex" "$FAKEBIN/pi"
ln -sf "$FAKEBIN/codex" "$FAKEBIN/agy"

# Restart the batch — the detached worker will eventually fail in the fake agent,
# but createRun() runs before agent invocation and must produce a new run.
PATH="$FAKEBIN:$PATH" "$WO" restart -b1 > restart_output.txt 2>&1 || true
cat restart_output.txt
grep -qF "已重启批量任务 b1" restart_output.txt

# Poll for worker to create the new run (up to 10 seconds).
python3 - <<'PY'
import hashlib
import json
import os
import time

repo_path = os.getcwd()
repo_key = os.path.basename(repo_path).lower().replace(".", "-") + "-" + hashlib.sha1(repo_path.encode()).hexdigest()[:10]
base = os.path.join(os.environ["XDG_STATE_HOME"], "wo", "repos", repo_key)
batches_dir = os.path.join(base, "batches")
runs_dir = os.path.join(base, "runs")
OLD_RUN_ID = "20260517T030000.000000000Z"
BATCH_ID = "20260517T030000.500000000Z"

deadline = time.time() + 10
batch = None
new_run_id = ""
while time.time() < deadline:
    batch_path = os.path.join(batches_dir, BATCH_ID, "state.json")
    if os.path.exists(batch_path):
        try:
            with open(batch_path, encoding="utf-8") as f:
                batch = json.load(f)
        except json.JSONDecodeError:
            time.sleep(0.3)
            continue
        new_run_id = batch["run_ids"].get("1-重启变更", "")
        if new_run_id and new_run_id != OLD_RUN_ID:
            break
    time.sleep(0.3)

assert batch is not None, "batch state not found after restart"

# 1. Old run association must be cleared.
assert new_run_id != OLD_RUN_ID, f"old run_id should have been cleared, got {new_run_id}"

# 2. current_index should be preserved.
assert batch["current_index"] == 0, f"expected current_index 0, got {batch['current_index']}"

# 3. Old run should still exist on disk (not deleted, just disassociated).
assert os.path.exists(os.path.join(runs_dir, OLD_RUN_ID, "state.json")), "old run directory should be preserved"

# 4. Detached worker must have created a new run for the current change.
assert new_run_id != "", "batch worker did not create a new run for '1-重启变更'"
assert os.path.exists(os.path.join(runs_dir, new_run_id, "state.json")), \
    f"new run state should exist at {new_run_id}"

# 5. Verify the new run state has correct change_name and batch_id.
with open(os.path.join(runs_dir, new_run_id, "state.json"), encoding="utf-8") as f:
    new_run = json.load(f)
assert new_run.get("change_name") == "1-重启变更", \
    f"new run change_name = {new_run.get('change_name')}"
assert new_run.get("batch_id") == BATCH_ID, \
    f"new run batch_id = {new_run.get('batch_id')}"

print("batch state valid after restart")
print(f"  status={batch['status']}, run_ids={batch['run_ids']}, current_index={batch['current_index']}")
print(f"  new_run_id={new_run_id}, new_run_status={new_run.get('status')}")
PY

echo "PASS"
