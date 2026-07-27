#!/usr/bin/env bash
# 文件功能目的：验证 oz flow 对需要持久产物的阶段使用同一角色会话重试修正，而 execution 以成功返回和后续门禁判定完成。
# Sources: 6-统一-oz-flow-阶段产物门禁重试并修复-parallel-artifact-合同, 47-移除并禁止提案任务文件
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

OZ_BIN="$TMP/wo"
note "build real oz flow binary"
(cd "$ROOT" && go build -o "$OZ_BIN" ./cmd/oz) >>"$RESULT_DIR/contract.log" 2>&1

FAKEBIN="$TMP/fakebin"
mkdir -p "$FAKEBIN"

cat >"$FAKEBIN/oz" <<'SH'
#!/usr/bin/env bash
# 文件功能目的：为临时仓库提供真实 oz flow 调用所需的最小 oz JSON 接口。
set -euo pipefail

case "$1" in
  list)
    printf '{"changes":[{"name":"1-stage-artifact-retry"}]}\n'
    ;;
  validate)
    printf '{"valid":true,"errors":[],"warnings":[],"artifacts":{}}\n'
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

states = sorted((state_home / "oz" / "flow" / "repos").glob("*/runs/*/state.json"))
if not states:
    raise SystemExit("no oz flow state found")
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
if role is None and (stage.startswith("audit_") or stage.startswith("targeted_repair_")):
    role = "repairer"
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

acceptance_path = repo / "docs" / "changes" / change / "acceptance.json"

def repair_needs_more(path):
    path.write_text(json.dumps({
        "summary": "已修复运行时证据，下一轮继续确认",
        "decision": "needs_more",
        "findings": [{
            "title": "需要复核运行时证据",
            "severity": "major",
            "scope": "current_change",
            "evidence": "fake runtime trace was repaired and requires a fresh verification",
            "recommendation": "下一轮重新运行验证"
        }],
        "non_blocking_findings": [],
        "evidence": ["runtime evidence repaired in the same repairer session"],
        "checks": {
            "oz_aligned": True,
            "tests_meaningful": True,
            "implementation_scoped": True,
            "runtime_behavior_verified": True,
            "previous_findings_resolved": False
        }
    }, ensure_ascii=False), encoding="utf-8")

def repair_clean(path):
    path.write_text(json.dumps({
        "summary": "同一会话完成修正与复核",
        "decision": "clean",
        "findings": [],
        "non_blocking_findings": [],
        "checks": {
            "oz_aligned": True,
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

def qa_needs_fix(path):
    """写入覆盖完整 acceptance matrix 的 QA 打回产物，驱动真实定向修复阶段。"""
    path.write_text(json.dumps({
        "summary": "QA 发现需要定向修复的运行时问题",
        "decision": "needs_fix",
        "findings": [{
            "title": "修复定向运行时证据",
            "severity": "major",
            "scope": "current_change",
            "evidence": "targeted runtime evidence is stale",
            "recommendation": "更新证据并复跑完整验收"
        }],
        "acceptance_matrix": [
            {
                "id": "contract-demo",
                "status": "failed",
                "artifact": "docs/changes/1-stage-artifact-retry/tests/demo.sh",
                "evidence": "contract requires one targeted repair"
            },
            {
                "id": "runtime-demo",
                "status": "passed",
                "artifact": "test-results/demo.zip",
                "evidence": "runtime trace remains available"
            }
        ],
        "evidence": [
            "runtime evidence: Playwright trace test-results/demo.zip"
        ]
    }, ensure_ascii=False), encoding="utf-8")

def archive_change(write_delivery):
    archive = repo / "docs" / "changes" / "archive" / ("2026-06-09-" + change)
    archive.parent.mkdir(parents=True, exist_ok=True)
    active = repo / "docs" / "changes" / change
    if active.exists() and not archive.exists():
        shutil.move(str(active), str(archive))
    if write_delivery:
        (run_dir / "delivery-summary.md").write_text("archive completed after artifact gate retry\n", encoding="utf-8")

if stage == "audit_1":
    if attempt >= 2:
        repair_needs_more(run_dir / "audit-1.json")
elif stage == "audit_2":
    if attempt == 1:
        (run_dir / "audit-2.json").write_text(json.dumps({
            "summary": "非法 severity",
            "decision": "needs_more",
            "findings": [{
                "title": "非法 severity",
                "severity": "urgent-info",
                "scope": "current_change",
                "evidence": "fake invalid severity",
                "recommendation": "重写为合法 severity"
            }],
            "non_blocking_findings": [],
            "evidence": ["invalid repair artifact for gate retry"],
            "checks": {}
        }, ensure_ascii=False), encoding="utf-8")
    else:
        repair_clean(run_dir / "audit-2.json")
elif stage == "qa_1":
    if attempt == 1:
        (run_dir / "qa-1.json").write_text(json.dumps({
            "summary": "QA 缺少 acceptance matrix",
            "decision": "clean",
            "findings": [],
            "acceptance_matrix": [],
            "evidence": ["runtime evidence: Playwright trace test-results/demo.zip"]
        }, ensure_ascii=False), encoding="utf-8")
    else:
        qa_needs_fix(run_dir / "qa-1.json")
elif stage == "targeted_repair_1":
    if attempt >= 2:
        (repo / "README.md").write_text("targeted repair completed\n", encoding="utf-8")
        repair_clean(run_dir / "targeted-repair-1.json")
elif stage == "qa_2":
    qa_clean(run_dir / "qa-2.json")
elif stage == "archive":
    archive_change(write_delivery=attempt >= 2)

print(json.dumps({"type": "thread.started", "thread_id": "thread-" + role}))
PY
SH
chmod +x "$FAKEBIN/codex"

cat >"$FAKEBIN/pi" <<'SH'
#!/usr/bin/env bash
# 文件功能目的：满足 oz flow agent preflight；本合同禁用 parallel，不会实际调用 pi。
set -euo pipefail
printf '{"type":"thread.started","thread_id":"unused-pi"}\n'
SH
chmod +x "$FAKEBIN/pi"

cat >"$FAKEBIN/agy" <<'SH'
#!/usr/bin/env bash
# 文件功能目的：满足 oz flow agent preflight；本合同禁用 agy，不会实际调用。
set -euo pipefail
printf '{"type":"thread.started","thread_id":"unused-agy"}\n'
SH
chmod +x "$FAKEBIN/agy"

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

cat >"$PROJECT/docs/changes/1-stage-artifact-retry/tests/demo.sh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
test -f docs/changes/1-stage-artifact-retry/acceptance.json
mkdir -p test-results
printf 'runtime trace fixture\n' > test-results/demo.zip
SH
chmod +x "$PROJECT/docs/changes/1-stage-artifact-retry/tests/demo.sh"

cat >"$PROJECT/docs/changes/1-stage-artifact-retry/acceptance.json" <<'JSON'
{
  "summary": "stage artifact retry acceptance",
  "coverage": [
    {
      "spec": "阶段产物重试",
      "tests": ["contract-demo"],
      "evidence": ["runtime-demo"],
      "risk": "covered by shell workflow contract"
    }
  ],
  "required_tests": [
    {
      "id": "contract-demo",
      "source": "change_contract",
      "path": "docs/changes/1-stage-artifact-retry/tests/demo.sh",
      "command": "bash docs/changes/1-stage-artifact-retry/tests/demo.sh",
      "purpose": "prove change test entry exists",
      "assertions": ["demo acceptance file exists and test entry produces test-results/demo.zip evidence"]
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

cat >"$PROJECT/oz-flow.yaml" <<'YAML'
max_repair_iterations: 2
validation:
  limit: 3
  commands: []
stages:
  execution:
    agent: codex
  repair:
    agent: codex
  qa:
    agent: codex
  archive:
    agent: codex
YAML

git -C "$PROJECT" add .
git -C "$PROJECT" commit -q -m initial

note "run oz flow and expect artifact-producing stage problems to be repaired in-session"
set +e
CODEX_ATTEMPT_DIR="$TMP/attempts" \
CODEX_CALL_LOG="$RESULT_DIR/codex-calls.jsonl" \
XDG_STATE_HOME="$TMP/state" \
HOME="$TMP/home" \
PATH="$FAKEBIN:/usr/bin:/bin" \
  bash -c 'cd "$1" && "$2" flow run --change "1-stage-artifact-retry" --json' _ "$PROJECT" "$OZ_BIN" >"$RESULT_DIR/run.jsonl" 2>"$RESULT_DIR/run.err"
run_code=$?
set -e
cat "$RESULT_DIR/run.jsonl" >>"$RESULT_DIR/contract.log"
cat "$RESULT_DIR/run.err" >>"$RESULT_DIR/contract.log"
[[ "$run_code" -eq 0 ]] || fail "oz flow run should repair missing/invalid stage artifacts instead of failing"

python3 - "$TMP/state" "$RESULT_DIR/codex-calls.jsonl" <<'PY' || exit 1
import json
import pathlib
import sys

state_home = pathlib.Path(sys.argv[1])
call_log = pathlib.Path(sys.argv[2])
states = sorted((state_home / "oz" / "flow" / "repos").glob("*/runs/*/state.json"))
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
    "audit_1": "thread-repairer",
    "audit_2": "thread-repairer",
    "qa_1": "thread-qa",
    "targeted_repair_1": "thread-repairer",
    "archive": "thread-archiver",
}
execution_attempts = by_stage.get("execution", [])
if len(execution_attempts) != 1 or execution_attempts[0].get("has_artifact_gate_prompt"):
    raise SystemExit(f"execution should complete once without a file artifact retry: {execution_attempts}")
for stage, session in required_retry.items():
    attempts = by_stage.get(stage, [])
    if len(attempts) < 2:
        raise SystemExit(f"{stage} attempts = {len(attempts)}, want retry")
    retry = attempts[1]
    if retry.get("session") != session:
        raise SystemExit(f"{stage} retry session = {retry.get('session')!r}, want {session!r}")
    if not retry.get("has_artifact_gate_prompt"):
        raise SystemExit(f"{stage} retry prompt did not include Stage artifact gate failed")

repair_sessions = {
    record.get("session")
    for record in records
    if record.get("stage") in {"audit_1", "audit_2", "targeted_repair_1"} and record.get("session")
}
if repair_sessions != {"thread-repairer"}:
    raise SystemExit(f"audit and targeted repair stages did not reuse one repairer session: {repair_sessions}")
required_repair_artifacts = [
    run_dir / "audit-1.json",
    run_dir / "audit-2.json",
    run_dir / "targeted-repair-1.json",
]
if not all(path.is_file() for path in required_repair_artifacts):
    raise SystemExit(f"missing durable quality-loop repair checkpoints: {required_repair_artifacts}")
qa_2_attempts = by_stage.get("qa_2", [])
if len(qa_2_attempts) != 1:
    raise SystemExit(f"qa_2 attempts = {len(qa_2_attempts)}, want one clean attempt")
if qa_2_attempts[0].get("session"):
    raise SystemExit("qa_2 must start in a fresh isolated QA session")
if any((run_dir / name).exists() for name in ("review-1.json", "review-2.json", "fix-1-summary.md")):
    raise SystemExit("new repair run must not emit legacy review/fix artifacts")

validation_files = sorted(run_dir.glob("validation-*.json"))
if len(validation_files) < 6:
    raise SystemExit(f"validation artifact count = {len(validation_files)}, want at least 6")
print("stage artifact gate retry assertions passed")
PY

archive_dir="$(find "$PROJECT/docs/changes/archive" -path '*-1-stage-artifact-retry' -type d -print -quit || true)"
if [[ -z "$archive_dir" ]]; then
  fail "archive directory missing after archive retry"
fi

note "contract passed: artifact-producing stages retry in-session without an execution task file"
