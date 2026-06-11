#!/usr/bin/env bash
# 文件功能目的：验证 go-dag subagent 正常退出但 artifact 字段类型错误时会 resume 原会话修正。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/go-dag/subagent-artifact-retry"
TMP="$(mktemp -d)"

cleanup() {
  rm -rf "$TMP"
}

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

note() {
  printf '%s\n' "$*" | tee -a "$RESULT_DIR/test.log"
}

trap cleanup EXIT
rm -rf "$RESULT_DIR"
mkdir -p "$RESULT_DIR"

WO_BIN="$TMP/wo"
OZ_BIN="$TMP/oz"
note "build real wo binary"
(cd "$ROOT" && go build -o "$WO_BIN" ./cmd/wo) >>"$RESULT_DIR/test.log" 2>&1
(cd "$ROOT" && go build -o "$OZ_BIN" ./cmd/oz) >>"$RESULT_DIR/test.log" 2>&1

FAKEBIN="$TMP/fakebin"
mkdir -p "$FAKEBIN"
ln -s "$OZ_BIN" "$FAKEBIN/oz"

cat >"$FAKEBIN/codex" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
python3 - <<'PY'
import json
import os
import pathlib

state_home = pathlib.Path(os.environ["XDG_STATE_HOME"])
states = sorted(state_home.glob("wo/repos/*/runs/*/state.json"))
if not states:
    raise SystemExit("no state.json found")
state_path = states[-1]
state = json.loads(state_path.read_text(encoding="utf-8"))
repo = pathlib.Path(os.environ["WO_TEST_REPO"])
run_dir = state_path.parent
change = state["change_name"]
stage = state["stage"]

if stage == "execution":
    task = repo / "docs" / "changes" / change / "task.md"
    task.write_text(task.read_text(encoding="utf-8").replace("- [ ]", "- [x]"), encoding="utf-8")
elif stage == "archive":
    archive = repo / "docs" / "changes" / "archive" / ("2026-06-09-" + change)
    archive.mkdir(parents=True, exist_ok=True)
    (archive / "acceptance.json").write_text((repo / "docs" / "changes" / change / "acceptance.json").read_text(encoding="utf-8"), encoding="utf-8")
    (run_dir / "delivery-summary.md").write_text("archive completed\n", encoding="utf-8")

print(json.dumps({"type": "thread.started", "thread_id": "codex-" + stage}))
PY
SH
chmod +x "$FAKEBIN/codex"
cp "$FAKEBIN/codex" "$FAKEBIN/legacy-agent"

cat >"$FAKEBIN/pi" <<'SH'
#!/usr/bin/env bash
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

printf 'session=%s\n%s\n---\n' "$session" "$prompt" >>"${PI_PROMPT_LOG:?}"

python3 - "$prompt" "$session" <<'PY'
import json
import os
import pathlib
import re
import sys

prompt, session = sys.argv[1:3]
output_match = re.search(r"^SUBAGENT_OUTPUT=(.+)$", prompt, re.M)
if not output_match:
    print(json.dumps({"type": "session", "id": "pi-main-session"}))
    raise SystemExit(0)

output = pathlib.Path(output_match.group(1).strip())
name = re.search(r"^SUBAGENT_NAME=(.+)$", prompt, re.M).group(1).strip()
purpose = re.search(r"^SUBAGENT_PURPOSE=(.+)$", prompt, re.M).group(1).strip()
change_name = re.search(r"^CURRENT_CHANGE=(.+)$", prompt, re.M).group(1).strip()
count_path = pathlib.Path(os.environ["PI_ATTEMPT_FILE"])
attempt = int(count_path.read_text(encoding="utf-8")) + 1 if count_path.exists() else 1
count_path.write_text(str(attempt), encoding="utf-8")
output.parent.mkdir(parents=True, exist_ok=True)

if attempt == 1:
    mutate_first = os.environ.get("PI_MUTATE_FIRST_FILE")
    if mutate_first:
        pathlib.Path(mutate_first).write_text("unexpected subagent source change\n", encoding="utf-8")
    body = {
        "name": name,
        "change_name": change_name,
        "purpose": purpose,
        "status": "success",
        "summary": "first artifact has malformed evidence",
        "evidence": [{"source": "go.mod", "location": "go.mod", "detail": "module"}],
    }
elif session == "pi-subagent-session" and ("evidence" in prompt and ("string" in prompt.lower() or "字符串数组" in prompt)):
    body = {
        "name": name,
        "change_name": change_name,
        "purpose": purpose,
        "status": "success",
        "summary": "artifact repaired in the same session",
        "evidence": ["go.mod: module github.com/xbugs221/oz"],
    }
else:
    raise SystemExit("retry did not resume the original session with schema guidance")

output.write_text(json.dumps(body, ensure_ascii=False), encoding="utf-8")
print(json.dumps({"type": "session", "id": "pi-subagent-session"}))
PY
SH
chmod +x "$FAKEBIN/pi"

PROJECT="$TMP/project"
mkdir -p "$PROJECT/docs/changes/1-子代理artifact重试/tests"
(
  cd "$PROJECT"
  git init -q
  git config user.email "test@example.com"
  git config user.name "Test User"
)

cat >"$PROJECT/docs/changes/1-子代理artifact重试/proposal.md" <<'MD'
# subagent artifact retry

## 问题

验证 go-dag subagent artifact schema retry。
MD

cat >"$PROJECT/docs/changes/1-子代理artifact重试/brief.md" <<'MD'
# subagent artifact retry

验证 go-dag 对 subagent artifact schema 错误执行同会话重试，并在只读边界被破坏时停止。
MD

cat >"$PROJECT/docs/changes/1-子代理artifact重试/design.md" <<'MD'
# 设计

使用 fake pi 产生一次格式错误，再由 wo resume 原会话修正。
MD

cat >"$PROJECT/docs/changes/1-子代理artifact重试/spec.md" <<'MD'
# 规格

### 需求：subagent artifact retry

系统必须修正 subagent artifact schema 错误。

#### 场景：同会话修复 artifact

- **当** subagent 首次写出 schema 错误 artifact
- **则** go-dag 必须复用原 subagent session 重试
- **且** 修复后的 fan-in artifact 必须继续推进
MD

cat >"$PROJECT/docs/changes/1-子代理artifact重试/task.md" <<'MD'
# 任务

- [ ] 1.1 完成 subagent artifact retry 验证
MD

cat >"$PROJECT/docs/changes/1-子代理artifact重试/tests/test_smoke.sh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
test -f docs/changes/1-子代理artifact重试/acceptance.json
SH
chmod +x "$PROJECT/docs/changes/1-子代理artifact重试/tests/test_smoke.sh"

cat >"$PROJECT/docs/changes/1-子代理artifact重试/acceptance.json" <<'JSON'
{
  "summary": "subagent artifact retry acceptance",
  "required_tests": [
    {
      "id": "smoke",
      "source": "change_contract",
      "path": "docs/changes/1-子代理artifact重试/tests/test_smoke.sh",
      "command": "bash docs/changes/1-子代理artifact重试/tests/test_smoke.sh",
      "purpose": "prove change test entry exists",
      "assertions": ["subagent artifact schema retry resumes the same session and repairs evidence"]
    }
  ],
  "required_evidence": [
    {
      "id": "runtime",
      "kind": "runtime_log",
      "path": "test-results/subagent-artifact-retry.log",
      "purpose": "record runtime retry behavior"
    }
  ]
}
JSON

cat >"$PROJECT/wo.yaml" <<'YAML'
wo:
  workflow:
    max_review_iterations: 0
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
PROJECT_READONLY="$TMP/project-readonly"
cp -a "$PROJECT" "$PROJECT_READONLY"

note "run default go-dag workflow and expect subagent artifact retry"
set +e
PI_PROMPT_LOG="$RESULT_DIR/pi-prompts.log" \
PI_ATTEMPT_FILE="$TMP/pi-attempts" \
WO_TEST_REPO="$PROJECT" \
XDG_STATE_HOME="$TMP/state" \
HOME="$TMP/home" \
PATH="$FAKEBIN:/usr/bin:/bin" \
  bash -c 'cd "$1" && "$2" run --change "1-子代理artifact重试" --json' _ "$PROJECT" "$WO_BIN" >"$RESULT_DIR/run.jsonl" 2>"$RESULT_DIR/run.err"
run_code=$?
set -e
cat "$RESULT_DIR/run.jsonl" >>"$RESULT_DIR/test.log"
cat "$RESULT_DIR/run.err" >>"$RESULT_DIR/test.log"
[[ "$run_code" -eq 0 ]] || fail "go-dag run should repair malformed subagent artifact instead of failing"

attempts="$(cat "$TMP/pi-attempts")"
[[ "$attempts" == "2" ]] || fail "expected exactly two pi subagent attempts, got $attempts"
grep -q 'session=pi-subagent-session' "$RESULT_DIR/pi-prompts.log" || fail "retry must resume the original pi subagent session"
grep -Eq 'evidence|字符串数组|string' "$RESULT_DIR/pi-prompts.log" || fail "retry prompt must include schema guidance for evidence"
grep -q 'SUBAGENT_OUTPUT' "$RESULT_DIR/pi-prompts.log" || fail "retry prompt must name the artifact output path"
grep -Eq '只重写|重写|rewrite' "$RESULT_DIR/pi-prompts.log" || fail "retry prompt must constrain the agent to rewrite only the artifact"

state="$(find "$TMP/state/wo/repos" -name state.json -type f -print | sort | tail -n 1)"
test -n "$state" || fail "missing state.json"
run_dir="$(dirname "$state")"
member_artifact="$(find "$run_dir/parallel-members/planning_context" -name '*.json' -type f -print | head -n 1)"
test -n "$member_artifact" || fail "missing planning_context member artifact"
python3 - "$member_artifact" <<'PY' || exit 1
import json
import sys
artifact = json.load(open(sys.argv[1], encoding="utf-8"))
if artifact.get("summary") != "artifact repaired in the same session":
    raise SystemExit("member artifact was not repaired")
if not isinstance(artifact.get("evidence"), list) or not all(isinstance(item, str) for item in artifact["evidence"]):
    raise SystemExit("member evidence must be a string array after repair")
PY

test -s "$run_dir/parallel-planning-context.json" || fail "fanin should continue after repaired member artifact"
note "contract passed: go-dag subagent artifact schema retry resumes the same session"

note "run go-dag workflow and expect read-only boundary failure before artifact retry"
set +e
PI_PROMPT_LOG="$RESULT_DIR/pi-readonly-prompts.log" \
PI_ATTEMPT_FILE="$TMP/pi-readonly-attempts" \
PI_MUTATE_FIRST_FILE="$PROJECT_READONLY/unexpected-source-change.txt" \
WO_TEST_REPO="$PROJECT_READONLY" \
XDG_STATE_HOME="$TMP/state-readonly" \
HOME="$TMP/home-readonly" \
PATH="$FAKEBIN:/usr/bin:/bin" \
  bash -c 'cd "$1" && "$2" run --change "1-子代理artifact重试" --json' _ "$PROJECT_READONLY" "$WO_BIN" >"$RESULT_DIR/readonly-run.jsonl" 2>"$RESULT_DIR/readonly-run.err"
readonly_code=$?
set -e
cat "$RESULT_DIR/readonly-run.jsonl" >>"$RESULT_DIR/test.log"
cat "$RESULT_DIR/readonly-run.err" >>"$RESULT_DIR/test.log"
[[ "$readonly_code" -ne 0 ]] || fail "go-dag run should fail when subagent mutates the worktree"
readonly_attempts="$(cat "$TMP/pi-readonly-attempts")"
[[ "$readonly_attempts" == "1" ]] || fail "read-only boundary must stop before artifact retry, got $readonly_attempts attempts"
grep -q '只读边界' "$RESULT_DIR/readonly-run.jsonl" "$RESULT_DIR/readonly-run.err" || fail "failure output must mention read-only boundary"
grep -q 'unexpected-source-change.txt' <(git -C "$PROJECT_READONLY" status --porcelain) || fail "fake pi should have created an unexpected worktree change"
note "contract passed: subagent read-only boundary is checked after each attempt"
