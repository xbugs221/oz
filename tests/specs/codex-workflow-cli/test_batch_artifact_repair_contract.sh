#!/usr/bin/env bash
# 文件功能目的：验证 batch 中某个 change 的 execution 产物可修复时，wo 会同会话修正并继续后续 change。
# Sources: 6-统一-wo-阶段产物门禁重试并修复-parallel-artifact-合同
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/6-stage-artifact-gate/batch-artifact-repair"
TMP="$(mktemp -d)"

# cleanup 清理本测试临时仓库和用户状态目录。
cleanup() {
  rm -rf "$TMP"
}

# fail 统一输出业务断言失败原因。
fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

# note 记录测试关键步骤，作为 acceptance runtime_log。
note() {
  printf '%s\n' "$*" | tee -a "$RESULT_DIR/contract.log"
}

trap cleanup EXIT
rm -rf "$RESULT_DIR"
mkdir -p "$RESULT_DIR"

WO_BIN="$TMP/wo"
note "build real wo binary"
(cd "$ROOT" && go build -o "$WO_BIN" ./cmd/wo) >>"$RESULT_DIR/contract.log" 2>&1

FAKEBIN="$TMP/fakebin"
mkdir -p "$FAKEBIN"

cat >"$FAKEBIN/oz" <<'SH'
#!/usr/bin/env bash
# 文件功能目的：为 batch 测试提供 change 校验和 task 完成状态。
set -euo pipefail

case "$1" in
  list)
    printf '{"changes":[{"name":"1-a"},{"name":"2-b"}]}\n'
    ;;
  validate)
    printf '{"valid":true,"errors":[],"warnings":[],"artifacts":{}}\n'
    ;;
  status)
    change="$2"
    if grep -q '\[x\]' "docs/changes/$change/task.md"; then
      printf '{"change":"%s","status":"ready","tasks":{"total":1,"done":1}}\n' "$change"
    else
      printf '{"change":"%s","status":"incomplete","tasks":{"total":1,"done":0}}\n' "$change"
    fi
    ;;
  *)
    printf 'unexpected oz command: %s\n' "$*" >&2
    exit 2
    ;;
esac
SH
chmod +x "$FAKEBIN/oz"

cat >"$FAKEBIN/codex" <<'SH'
#!/usr/bin/env bash
# 文件功能目的：第一个 change 的 execution 首次不完成 task，验证 batch 等待 artifact gate retry。
set -euo pipefail

repo=""
session=""
while (($#)); do
  case "$1" in
    --cd)
      repo="$2"
      shift 2
      ;;
    resume)
      session="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
prompt="$(cat)"

CODEX_REPO="$repo" \
CODEX_SESSION="$session" \
CODEX_PROMPT="$prompt" \
python3 - <<'PY'
import json
import os
import pathlib
import re
import shutil

repo = pathlib.Path(os.environ["CODEX_REPO"])
session = os.environ["CODEX_SESSION"]
prompt = os.environ["CODEX_PROMPT"]
state_home = pathlib.Path(os.environ["XDG_STATE_HOME"])
attempt_dir = pathlib.Path(os.environ["CODEX_ATTEMPT_DIR"])
call_log = pathlib.Path(os.environ["CODEX_CALL_LOG"])

states = sorted((state_home / "wo" / "repos").glob("*/runs/*/state.json"))
running = []
for path in states:
    state = json.loads(path.read_text(encoding="utf-8"))
    if state.get("status") == "running":
        running.append((path, state))
if not running:
    raise SystemExit("no running state found")
state_path, state = running[-1]
run_dir = state_path.parent
stage = state["stage"]
change = state["change_name"]
role = "archiver" if stage == "archive" else "executor"

attempt_dir.mkdir(parents=True, exist_ok=True)
key = re.sub(r"[^A-Za-z0-9_.-]+", "_", f"{change}__{stage}")
attempt_path = attempt_dir / key
attempt = int(attempt_path.read_text(encoding="utf-8")) + 1 if attempt_path.exists() else 1
attempt_path.write_text(str(attempt), encoding="utf-8")

call_log.parent.mkdir(parents=True, exist_ok=True)
with call_log.open("a", encoding="utf-8") as fh:
    fh.write(json.dumps({
        "change": change,
        "stage": stage,
        "attempt": attempt,
        "session": session,
        "has_artifact_gate_prompt": "Stage artifact gate failed" in prompt,
    }, ensure_ascii=False) + "\n")

task_path = repo / "docs" / "changes" / change / "task.md"
acceptance_path = repo / "docs" / "changes" / change / "acceptance.json"
if stage == "execution":
    if change != "1-a" or attempt >= 2:
        text = task_path.read_text(encoding="utf-8")
        task_path.write_text(text.replace("- [ ]", "- [x]"), encoding="utf-8")
elif stage == "archive":
    archive = repo / "docs" / "changes" / "archive" / ("2026-06-09-" + change)
    archive.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(acceptance_path, archive / "acceptance.json")
    (run_dir / "delivery-summary.md").write_text(f"archived {change}\n", encoding="utf-8")

print(json.dumps({"type": "thread.started", "thread_id": "thread-" + role}))
PY
SH
chmod +x "$FAKEBIN/codex"

PROJECT="$TMP/project"
mkdir -p "$PROJECT/docs/changes/1-a/tests" "$PROJECT/docs/changes/2-b/tests"
(
  cd "$PROJECT"
  git init -q
  git config user.email test@example.com
  git config user.name "Test User"
)

for change in 1-a 2-b; do
  cat >"$PROJECT/docs/changes/$change/proposal.md" <<MD
# $change

## 问题

验证 batch artifact gate retry。
MD
  cat >"$PROJECT/docs/changes/$change/design.md" <<'MD'
# 设计

使用 fake codex 验证 batch 继续执行。
MD
  cat >"$PROJECT/docs/changes/$change/spec.md" <<'MD'
# 规格

### 需求：batch artifact gate retry

系统必须修复当前 change 后继续 batch。
MD
  cat >"$PROJECT/docs/changes/$change/task.md" <<'MD'
# 任务

- [ ] 1.1 完成 batch 验证
MD
  cat >"$PROJECT/docs/changes/$change/tests/demo.sh" <<SH
#!/usr/bin/env bash
set -euo pipefail
test -f docs/changes/$change/acceptance.json
SH
  chmod +x "$PROJECT/docs/changes/$change/tests/demo.sh"
  cat >"$PROJECT/docs/changes/$change/acceptance.json" <<JSON
{
  "summary": "$change acceptance",
  "required_tests": [
    {
      "id": "contract-$change",
      "source": "change_contract",
      "path": "docs/changes/$change/tests/demo.sh",
      "command": "bash docs/changes/$change/tests/demo.sh",
      "purpose": "prove $change test entry exists"
    }
  ],
  "required_evidence": []
}
JSON
done

cat >"$PROJECT/wo.yaml" <<'YAML'
wo:
  workflow:
    engine: go-dag
    max_review_iterations: 0
    validation:
      max_attempts_per_stage: 3
      commands: []
    parallel:
      enabled: false
    stages:
      execution:
        tool: codex
      archive:
        tool: codex
YAML

git -C "$PROJECT" add .
git -C "$PROJECT" commit -q -m initial

repo_key="$(python3 - "$PROJECT" <<'PY'
import hashlib
import os
import sys

repo = os.path.abspath(sys.argv[1])
print(os.path.basename(repo).lower() + "-" + hashlib.sha1(repo.encode()).hexdigest()[:10])
PY
)"
batch_dir="$TMP/state/wo/repos/$repo_key/batches/batch-artifact-retry"
mkdir -p "$batch_dir"
cat >"$batch_dir/state.json" <<'JSON'
{
  "batch_id": "batch-artifact-retry",
  "status": "running",
  "changes": ["1-a", "2-b"],
  "current_index": 0,
  "run_ids": {},
  "error": ""
}
JSON

note "run batch and expect first change execution artifact repair before second change"
set +e
CODEX_ATTEMPT_DIR="$TMP/attempts" \
CODEX_CALL_LOG="$RESULT_DIR/codex-calls.jsonl" \
XDG_STATE_HOME="$TMP/state" \
HOME="$TMP/home" \
PATH="$FAKEBIN:/usr/bin:/bin" \
  bash -c 'cd "$1" && "$2" batch --batch-id batch-artifact-retry --json' _ "$PROJECT" "$WO_BIN" >"$RESULT_DIR/batch.jsonl" 2>"$RESULT_DIR/batch.err"
batch_code=$?
set -e
cat "$RESULT_DIR/batch.jsonl" >>"$RESULT_DIR/contract.log"
cat "$RESULT_DIR/batch.err" >>"$RESULT_DIR/contract.log"
[[ "$batch_code" -eq 0 ]] || fail "batch should continue after repairing first execution artifact"

python3 - "$batch_dir/state.json" "$RESULT_DIR/codex-calls.jsonl" "$TMP/state" "$PROJECT" <<'PY' || exit 1
import json
import pathlib
import sys

batch_state = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
records = [json.loads(line) for line in pathlib.Path(sys.argv[2]).read_text(encoding="utf-8").splitlines() if line.strip()]
state_home = pathlib.Path(sys.argv[3])
project = pathlib.Path(sys.argv[4])

if batch_state.get("status") != "done":
    raise SystemExit(f"batch status = {batch_state.get('status')!r}, want done")
if batch_state.get("current_index") != 2:
    raise SystemExit(f"current_index = {batch_state.get('current_index')!r}, want 2")
if set(batch_state.get("run_ids", {}).keys()) != {"1-a", "2-b"}:
    raise SystemExit(f"run_ids = {batch_state.get('run_ids')!r}, want both changes")

first_execution = [r for r in records if r["change"] == "1-a" and r["stage"] == "execution"]
if len(first_execution) < 2:
    raise SystemExit("1-a execution did not retry")
retry = first_execution[1]
if retry.get("session") != "thread-executor":
    raise SystemExit(f"1-a execution retry session = {retry.get('session')!r}, want thread-executor")
if not retry.get("has_artifact_gate_prompt"):
    raise SystemExit("1-a execution retry prompt did not include artifact gate failure")

run_states = sorted((state_home / "wo" / "repos").glob("*/runs/*/state.json"))
if len(run_states) != 2:
    raise SystemExit(f"run state count = {len(run_states)}, want 2")
for path in run_states:
    state = json.loads(path.read_text(encoding="utf-8"))
    if state.get("status") != "done":
        raise SystemExit(f"{path} status = {state.get('status')!r}, want done")
for change in ["1-a", "2-b"]:
    task = (project / "docs" / "changes" / change / "task.md").read_text(encoding="utf-8")
    if "[x]" not in task:
        raise SystemExit(f"{change} task was not completed")
print("batch artifact repair assertions passed")
PY

note "contract passed: batch continues after stage artifact repair"
