#!/usr/bin/env bash
# 文件功能目的：验证 wo 对每个主阶段的缺失或非法产物都使用同一角色会话重试修正，而不是直接失败。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/6-stage-artifact-gate/stage-artifact-gate-retry"
TMP="$(mktemp -d)"

# cleanup 删除本测试创建的临时仓库、fake CLI 和状态目录，避免污染维护者环境。
cleanup() {
  rm -rf "$TMP"
}

# fail 输出清晰失败原因，方便执行阶段从 runtime log 定位断言。
fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

# note 同时写入测试日志和 stdout，保留可复核执行步骤。
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
# 文件功能目的：为临时仓库提供真实 wo 调用所需的最小 oz JSON 接口。
set -euo pipefail

case "$1" in
  list)
    printf '{"changes":[{"name":"1-stage-artifact-retry"}]}\n'
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
# 文件功能目的：模拟主阶段 Codex，首次漏写或写坏阶段产物，重试时在同一 session 中修正。
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
if not states:
    raise SystemExit("no wo state found")
running = []
for path in states:
    try:
        state = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError:
        continue
    if state.get("status") == "running":
        running.append((path, state))
state_path, state = running[-1] if running else (states[-1], json.loads(states[-1].read_text(encoding="utf-8")))

run_dir = state_path.parent
stage = state["stage"]
change = state["change_name"]
role = {
    "execution": "executor",
    "archive": "archiver",
}.get(stage)
if role is None and stage.startswith("review_"):
    role = "reviewer"
elif role is None and stage.startswith("fix_"):
    role = "fixer"
elif role is None and stage.startswith("qa_"):
    role = "qa"
if role is None:
    role = stage

attempt_dir.mkdir(parents=True, exist_ok=True)
key = re.sub(r"[^A-Za-z0-9_.-]+", "_", f"{change}__{stage}")
attempt_path = attempt_dir / key
attempt = int(attempt_path.read_text(encoding="utf-8")) + 1 if attempt_path.exists() else 1
attempt_path.write_text(str(attempt), encoding="utf-8")

call_log.parent.mkdir(parents=True, exist_ok=True)
with call_log.open("a", encoding="utf-8") as fh:
    fh.write(json.dumps({
        "stage": stage,
        "role": role,
        "attempt": attempt,
        "session": session,
        "has_artifact_gate_prompt": "Stage artifact gate failed" in prompt,
    }, ensure_ascii=False) + "\n")

task_path = repo / "docs" / "changes" / change / "task.md"
acceptance_path = repo / "docs" / "changes" / change / "acceptance.json"

def mark_task_done():
    text = task_path.read_text(encoding="utf-8")
    task_path.write_text(text.replace("- [ ]", "- [x]"), encoding="utf-8")

def review_needs_fix(path):
    path.write_text(json.dumps({
        "summary": "需要修复运行时证据",
        "decision": "needs_fix",
        "findings": [{
            "title": "缺少运行时证据",
            "severity": "major",
            "evidence": "fake runtime trace is missing before fix",
            "recommendation": "补齐运行时证据后重新审核"
        }],
        "evidence": [],
        "workflow_failure": None,
        "checks": {
            "oz_aligned": False,
            "tasks_verified": True,
            "tests_meaningful": False,
            "implementation_scoped": True,
            "runtime_behavior_verified": False,
            "previous_findings_resolved": False
        }
    }, ensure_ascii=False), encoding="utf-8")

def review_clean(path):
    path.write_text(json.dumps({
        "summary": "修复后审核通过",
        "decision": "clean",
        "findings": [],
        "checks": {
            "oz_aligned": True,
            "tasks_verified": True,
            "tests_meaningful": True,
            "implementation_scoped": True,
            "runtime_behavior_verified": True,
            "previous_findings_resolved": True
        },
        "evidence": [
            "validation artifact passed: validation-execution-1.json",
            "runtime evidence: Playwright trace test-results/demo.zip"
        ]
    }, ensure_ascii=False), encoding="utf-8")

def qa_clean(path):
    path.write_text(json.dumps({
        "summary": "QA 证据完整",
        "decision": "clean",
        "findings": [],
        "acceptance_matrix": [
            {
                "id": "contract-demo",
                "status": "passed",
                "artifact": "docs/changes/1-stage-artifact-retry/tests/demo.sh",
                "evidence": "bash docs/changes/1-stage-artifact-retry/tests/demo.sh passed"
            },
            {
                "id": "runtime-demo",
                "status": "passed",
                "artifact": "test-results/demo.zip",
                "evidence": "runtime trace captured for demo path"
            }
        ],
        "evidence": [
            "runtime evidence: Playwright trace test-results/demo.zip"
        ]
    }, ensure_ascii=False), encoding="utf-8")

def archive_change(write_delivery):
    archive = repo / "docs" / "changes" / "archive" / ("2026-06-09-" + change)
    archive.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(acceptance_path, archive / "acceptance.json")
    if write_delivery:
        (run_dir / "delivery-summary.md").write_text("archive completed after artifact gate retry\n", encoding="utf-8")

if stage == "execution":
    if attempt >= 2:
        mark_task_done()
elif stage == "review_1":
    if attempt >= 2:
        review_needs_fix(run_dir / "review-1.json")
elif stage == "fix_1":
    if attempt >= 2:
        (run_dir / "fix-1-summary.md").write_text("修复运行时证据缺口\n", encoding="utf-8")
elif stage == "review_2":
    if attempt == 1:
        (run_dir / "review-2.json").write_text(json.dumps({
            "summary": "非法 severity",
            "decision": "needs_fix",
            "findings": [{
                "title": "非法 severity",
                "severity": "urgent-info",
                "evidence": "fake invalid severity",
                "recommendation": "重写为合法 severity"
            }],
            "evidence": [],
            "checks": {}
        }, ensure_ascii=False), encoding="utf-8")
    else:
        review_clean(run_dir / "review-2.json")
elif stage == "qa_2":
    if attempt == 1:
        (run_dir / "qa-2.json").write_text(json.dumps({
            "summary": "QA 缺少 acceptance matrix",
            "decision": "clean",
            "findings": [],
            "acceptance_matrix": [],
            "evidence": ["runtime evidence: Playwright trace test-results/demo.zip"]
        }, ensure_ascii=False), encoding="utf-8")
    else:
        qa_clean(run_dir / "qa-2.json")
elif stage == "archive":
    archive_change(write_delivery=attempt >= 2)

print(json.dumps({"type": "thread.started", "thread_id": "thread-" + role}))
PY
SH
chmod +x "$FAKEBIN/codex"

PROJECT="$TMP/project"
mkdir -p "$PROJECT/docs/changes/1-stage-artifact-retry/tests"
(
  cd "$PROJECT"
  git init -q
  git config user.email test@example.com
  git config user.name "Test User"
)

cat >"$PROJECT/docs/changes/1-stage-artifact-retry/proposal.md" <<'MD'
# stage artifact retry

## 问题

验证主阶段产物缺失和非法时会同会话重试。
MD

cat >"$PROJECT/docs/changes/1-stage-artifact-retry/design.md" <<'MD'
# 设计

使用 fake codex 稳定制造阶段产物问题。
MD

cat >"$PROJECT/docs/changes/1-stage-artifact-retry/spec.md" <<'MD'
# 规格

### 需求：阶段产物重试

系统必须同会话修正缺失或非法阶段产物。
MD

cat >"$PROJECT/docs/changes/1-stage-artifact-retry/task.md" <<'MD'
# 任务

- [ ] 1.1 完成阶段产物重试验证
MD

cat >"$PROJECT/docs/changes/1-stage-artifact-retry/tests/demo.sh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
test -f docs/changes/1-stage-artifact-retry/acceptance.json
SH
chmod +x "$PROJECT/docs/changes/1-stage-artifact-retry/tests/demo.sh"

cat >"$PROJECT/docs/changes/1-stage-artifact-retry/acceptance.json" <<'JSON'
{
  "summary": "stage artifact retry acceptance",
  "required_tests": [
    {
      "id": "contract-demo",
      "source": "change_contract",
      "path": "docs/changes/1-stage-artifact-retry/tests/demo.sh",
      "command": "bash docs/changes/1-stage-artifact-retry/tests/demo.sh",
      "purpose": "prove change test entry exists"
    }
  ],
  "required_evidence": [
    {
      "id": "runtime-demo",
      "kind": "runtime_log",
      "path": "test-results/demo.zip",
      "purpose": "record runtime QA evidence"
    }
  ]
}
JSON

cat >"$PROJECT/wo.yaml" <<'YAML'
wo:
  workflow:
    engine: go-dag
    max_review_iterations: 2
    validation:
      max_attempts_per_stage: 3
      commands: []
    parallel:
      enabled: false
    stages:
      execution:
        tool: codex
      review:
        tool: codex
      qa:
        tool: codex
      fix:
        tool: codex
      archive:
        tool: codex
YAML

git -C "$PROJECT" add .
git -C "$PROJECT" commit -q -m initial

note "run wo and expect every stage artifact problem to be repaired in-session"
set +e
CODEX_ATTEMPT_DIR="$TMP/attempts" \
CODEX_CALL_LOG="$RESULT_DIR/codex-calls.jsonl" \
XDG_STATE_HOME="$TMP/state" \
HOME="$TMP/home" \
PATH="$FAKEBIN:/usr/bin:/bin" \
  bash -c 'cd "$1" && "$2" run --change "1-stage-artifact-retry" --json' _ "$PROJECT" "$WO_BIN" >"$RESULT_DIR/run.jsonl" 2>"$RESULT_DIR/run.err"
run_code=$?
set -e
cat "$RESULT_DIR/run.jsonl" >>"$RESULT_DIR/contract.log"
cat "$RESULT_DIR/run.err" >>"$RESULT_DIR/contract.log"
[[ "$run_code" -eq 0 ]] || fail "wo run should repair missing/invalid stage artifacts instead of failing"

python3 - "$TMP/state" "$RESULT_DIR/codex-calls.jsonl" <<'PY' || exit 1
import json
import pathlib
import sys

state_home = pathlib.Path(sys.argv[1])
call_log = pathlib.Path(sys.argv[2])
states = sorted((state_home / "wo" / "repos").glob("*/runs/*/state.json"))
if not states:
    raise SystemExit("missing run state")
state_path = states[-1]
state = json.loads(state_path.read_text(encoding="utf-8"))
if state.get("status") != "done":
    raise SystemExit(f"run status = {state.get('status')!r}, want done")
run_dir = state_path.parent
if not (run_dir / "delivery-summary.md").is_file():
    raise SystemExit("missing delivery summary after archive retry")

records = [json.loads(line) for line in call_log.read_text(encoding="utf-8").splitlines() if line.strip()]
by_stage = {}
for record in records:
    by_stage.setdefault(record["stage"], []).append(record)

required_retry = {
    "execution": "thread-executor",
    "review_1": "thread-reviewer",
    "fix_1": "thread-fixer",
    "review_2": "thread-reviewer",
    "qa_2": "thread-qa",
    "archive": "thread-archiver",
}
for stage, session in required_retry.items():
    attempts = by_stage.get(stage, [])
    if len(attempts) < 2:
        raise SystemExit(f"{stage} attempts = {len(attempts)}, want retry")
    retry = attempts[1]
    if retry.get("session") != session:
        raise SystemExit(f"{stage} retry session = {retry.get('session')!r}, want {session!r}")
    if not retry.get("has_artifact_gate_prompt"):
        raise SystemExit(f"{stage} retry prompt did not include Stage artifact gate failed")

validation_files = sorted(run_dir.glob("validation-*.json"))
if len(validation_files) < 5:
    raise SystemExit(f"validation artifact count = {len(validation_files)}, want at least 5")
print("stage artifact gate retry assertions passed")
PY

archive_dir="$(find "$PROJECT/docs/changes/archive" -path '*-1-stage-artifact-retry' -type d -print -quit || true)"
if [[ -z "$archive_dir" ]]; then
  fail "archive directory missing after archive retry"
fi

note "contract passed: all main stage artifact problems are repaired through same-session retry"
