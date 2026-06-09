#!/usr/bin/env bash
# 文件功能目的：验证 parallel subagent 写出 severity=info 时会被归一化为 minor，不会中断 go-dag workflow。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/6-stage-artifact-gate/parallel-info-severity"
TMP="$(mktemp -d)"

# cleanup 删除临时工程、fake CLI 和 XDG 状态目录。
cleanup() {
  rm -rf "$TMP"
}

# fail 输出本测试的业务断言失败原因。
fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

# note 记录可复核的测试进度。
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
# 文件功能目的：为 parallel severity 契约测试提供 oz task 状态。
set -euo pipefail

case "$1" in
  list)
    printf '{"changes":[{"name":"1-parallel-info-severity"}]}\n'
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
# 文件功能目的：完成主 execution/archive 阶段，让测试聚焦 parallel subagent artifact 合同。
set -euo pipefail

repo=""
while (($#)); do
  case "$1" in
    --cd)
      repo="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
prompt="$(cat)"

CODEX_REPO="$repo" CODEX_PROMPT="$prompt" python3 - <<'PY'
import json
import os
import pathlib
import shutil

repo = pathlib.Path(os.environ["CODEX_REPO"])
state_home = pathlib.Path(os.environ["XDG_STATE_HOME"])
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
task_path = repo / "docs" / "changes" / change / "task.md"
acceptance_path = repo / "docs" / "changes" / change / "acceptance.json"

if stage == "execution":
    task_path.write_text(task_path.read_text(encoding="utf-8").replace("- [ ]", "- [x]"), encoding="utf-8")
elif stage == "archive":
    archive = repo / "docs" / "changes" / "archive" / ("2026-06-09-" + change)
    archive.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(acceptance_path, archive / "acceptance.json")
    (run_dir / "delivery-summary.md").write_text("archive completed\n", encoding="utf-8")

print(json.dumps({"type": "thread.started", "thread_id": "codex-" + stage}))
PY
SH
chmod +x "$FAKEBIN/codex"

cat >"$FAKEBIN/pi" <<'SH'
#!/usr/bin/env bash
# 文件功能目的：模拟只读 subagent 首次写出 severity=info，必要时在同一 session 中修正为 minor。
set -euo pipefail

session=""
prompt=""
while (($#)); do
  case "$1" in
    --session)
      session="$2"
      shift 2
      ;;
    --mode|--model|--thinking)
      shift 2
      ;;
    *)
      prompt="$1"
      shift
      ;;
  esac
done

PI_SESSION="$session" PI_PROMPT="$prompt" python3 - <<'PY'
import json
import os
import pathlib
import re

session = os.environ["PI_SESSION"]
prompt = os.environ["PI_PROMPT"]
attempt_file = pathlib.Path(os.environ["PI_ATTEMPT_FILE"])
prompt_log = pathlib.Path(os.environ["PI_PROMPT_LOG"])

output_match = re.search(r"^SUBAGENT_OUTPUT=(.+)$", prompt, re.M)
if not output_match:
    print(json.dumps({"type": "session", "id": "pi-main-session"}))
    raise SystemExit(0)

output = pathlib.Path(output_match.group(1).strip())
name = re.search(r"^SUBAGENT_NAME=(.+)$", prompt, re.M).group(1).strip()
purpose = re.search(r"^SUBAGENT_PURPOSE=(.+)$", prompt, re.M).group(1).strip()
attempt = int(attempt_file.read_text(encoding="utf-8")) + 1 if attempt_file.exists() else 1
attempt_file.write_text(str(attempt), encoding="utf-8")

prompt_log.parent.mkdir(parents=True, exist_ok=True)
with prompt_log.open("a", encoding="utf-8") as fh:
    fh.write(json.dumps({
        "attempt": attempt,
        "session": session,
        "has_output": "SUBAGENT_OUTPUT" in prompt,
        "has_severity_guidance": "severity" in prompt or "严重级别" in prompt,
    }, ensure_ascii=False) + "\n")

severity = "info"
if attempt > 1:
    if session != "pi-planning-session":
        raise SystemExit("retry did not resume the original pi subagent session")
    severity = "minor"

body = {
    "name": name,
    "purpose": purpose,
    "status": "success",
    "summary": "checked planning context and reported one informational finding",
    "evidence": ["docs/changes/1-parallel-info-severity/spec.md inspected"],
    "findings": [
        {
            "title": "提示性上下文差异",
            "severity": severity,
            "evidence": "fake pi found a non-blocking planning note",
            "recommendation": "记录为低风险上下文，不阻断 workflow"
        }
    ]
}
output.parent.mkdir(parents=True, exist_ok=True)
output.write_text(json.dumps(body, ensure_ascii=False), encoding="utf-8")
print(json.dumps({"type": "session", "id": "pi-planning-session"}))
PY
SH
chmod +x "$FAKEBIN/pi"

PROJECT="$TMP/project"
mkdir -p "$PROJECT/docs/changes/1-parallel-info-severity/tests"
(
  cd "$PROJECT"
  git init -q
  git config user.email test@example.com
  git config user.name "Test User"
)

cat >"$PROJECT/docs/changes/1-parallel-info-severity/proposal.md" <<'MD'
# parallel info severity

## 问题

验证 parallel subagent info severity 不会中断 workflow。
MD

cat >"$PROJECT/docs/changes/1-parallel-info-severity/design.md" <<'MD'
# 设计

使用 fake pi 产出 info severity。
MD

cat >"$PROJECT/docs/changes/1-parallel-info-severity/spec.md" <<'MD'
# 规格

### 需求：parallel severity 归一化

系统必须将 info severity 归一化为 minor。
MD

cat >"$PROJECT/docs/changes/1-parallel-info-severity/task.md" <<'MD'
# 任务

- [ ] 1.1 完成 parallel severity 验证
MD

cat >"$PROJECT/docs/changes/1-parallel-info-severity/tests/demo.sh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
test -f docs/changes/1-parallel-info-severity/acceptance.json
SH
chmod +x "$PROJECT/docs/changes/1-parallel-info-severity/tests/demo.sh"

cat >"$PROJECT/docs/changes/1-parallel-info-severity/acceptance.json" <<'JSON'
{
  "summary": "parallel severity acceptance",
  "required_tests": [
    {
      "id": "contract-demo",
      "source": "change_contract",
      "path": "docs/changes/1-parallel-info-severity/tests/demo.sh",
      "command": "bash docs/changes/1-parallel-info-severity/tests/demo.sh",
      "purpose": "prove change test entry exists"
    }
  ],
  "required_evidence": []
}
JSON

cat >"$PROJECT/wo.yaml" <<'YAML'
wo:
  workflow:
    engine: go-dag
    max_review_iterations: 0
    validation:
      max_attempts_per_stage: 3
      commands: []
    stages:
      execution:
        tool: codex
      archive:
        tool: codex
    parallel:
      enabled: true
      groups:
        planning_context:
          mode: advisory
          members:
            - name: 外部资料研究员
              purpose: 查询外部库文档和开源实现
              tool: pi
YAML

git -C "$PROJECT" add .
git -C "$PROJECT" commit -q -m initial

note "run wo and expect info severity to normalize instead of failing"
set +e
PI_ATTEMPT_FILE="$TMP/pi-attempts" \
PI_PROMPT_LOG="$RESULT_DIR/pi-prompts.jsonl" \
XDG_STATE_HOME="$TMP/state" \
HOME="$TMP/home" \
PATH="$FAKEBIN:/usr/bin:/bin" \
  bash -c 'cd "$1" && "$2" run --change "1-parallel-info-severity" --json' _ "$PROJECT" "$WO_BIN" >"$RESULT_DIR/run.jsonl" 2>"$RESULT_DIR/run.err"
run_code=$?
set -e
cat "$RESULT_DIR/run.jsonl" >>"$RESULT_DIR/contract.log"
cat "$RESULT_DIR/run.err" >>"$RESULT_DIR/contract.log"
[[ "$run_code" -eq 0 ]] || fail "wo run should normalize or repair parallel subagent severity=info instead of failing"

python3 - "$TMP/state" "$RESULT_DIR/pi-prompts.jsonl" <<'PY' || exit 1
import json
import pathlib
import sys

state_home = pathlib.Path(sys.argv[1])
prompt_log = pathlib.Path(sys.argv[2])
states = sorted((state_home / "wo" / "repos").glob("*/runs/*/state.json"))
if not states:
    raise SystemExit("missing run state")
state_path = states[-1]
state = json.loads(state_path.read_text(encoding="utf-8"))
if state.get("status") != "done":
    raise SystemExit(f"run status = {state.get('status')!r}, want done")
run_dir = state_path.parent

member_files = sorted((run_dir / "parallel-members" / "planning_context").glob("*.json"))
if len(member_files) != 1:
    raise SystemExit(f"member artifact count = {len(member_files)}, want 1")
member = json.loads(member_files[0].read_text(encoding="utf-8"))
group = json.loads((run_dir / "parallel-planning-context.json").read_text(encoding="utf-8"))

member_severity = member["findings"][0]["severity"]
group_severity = group["members"][0]["findings"][0]["severity"]
if member_severity != "minor":
    raise SystemExit(f"member severity = {member_severity!r}, want minor")
if group_severity != "minor":
    raise SystemExit(f"group severity = {group_severity!r}, want minor")

records = [json.loads(line) for line in prompt_log.read_text(encoding="utf-8").splitlines() if line.strip()]
if len(records) > 2:
    raise SystemExit(f"pi attempts = {len(records)}, want at most 2")
if len(records) == 2:
    retry = records[1]
    if retry.get("session") != "pi-planning-session":
        raise SystemExit(f"retry session = {retry.get('session')!r}, want pi-planning-session")
    if not retry.get("has_output"):
        raise SystemExit("retry prompt did not include SUBAGENT_OUTPUT")
print("parallel info severity assertions passed")
PY

note "contract passed: parallel subagent info severity is normalized and fan-in continues"
