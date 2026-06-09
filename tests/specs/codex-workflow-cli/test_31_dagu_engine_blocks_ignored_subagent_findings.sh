#!/usr/bin/env bash
# 文件功能目的：验证 Dagu engine 下 gate_input subagent 的 blocker finding 不能被 clean review 绕过。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/31-dagu-engine/blocked"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

rm -rf "$RESULT_DIR"
mkdir -p "$RESULT_DIR"

BIN="$TMP/wo"
go build -o "$BIN" "$ROOT/cmd/wo"

WORK="$TMP/work"
HOME_DIR="$TMP/home"
STATE_HOME="$TMP/state"
FAKEBIN="$TMP/fakebin"
mkdir -p "$WORK" "$HOME_DIR" "$STATE_HOME" "$FAKEBIN"
ln -s "$BIN" "$FAKEBIN/wo"

cd "$WORK"
git init >/dev/null
git config user.email test@example.com
git config user.name Test
printf 'demo\n' > README.md
mkdir -p docs/changes/demo
cat > docs/changes/demo/task.md <<'EOF'
- [ ] implement demo
EOF
cat > docs/changes/demo/acceptance.json <<'JSON'
{
  "summary": "demo acceptance",
  "required_tests": [
    {
      "id": "contract-demo",
      "source": "change_contract",
      "path": "docs/changes/demo/tests/demo.acceptance.test.ts",
      "command": "true",
      "purpose": "cover demo contract"
    }
  ],
  "required_evidence": []
}
JSON
git add .
git commit -m init >/dev/null

cat > "$FAKEBIN/oz" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  validate) printf '{"valid":true,"errors":[],"warnings":[],"artifacts":{}}\n' ;;
  status)
    if grep -q '\[x\]' docs/changes/demo/task.md; then
      printf '{"change":"demo","status":"ready","tasks":{"total":1,"done":1}}\n'
    else
      printf '{"change":"demo","status":"incomplete","tasks":{"total":1,"done":0}}\n'
    fi
    ;;
  list) printf '{"changes":[{"name":"demo"}]}\n' ;;
  *) printf 'unexpected oz command: %s\n' "$*" >&2; exit 2 ;;
esac
EOF
chmod +x "$FAKEBIN/oz"

cat > "$FAKEBIN/dagu" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'dagu args: %s\n' "$*" >> "${WO_DAGU_LOG:?}"
if [[ "${1:-}" != "start" ]]; then
  printf "expected dagu start <workflow.yaml>, got: %s\n" "$*" >&2
  exit 2
fi
shift
if [[ "$#" -ne 1 || ! -f "$1" ]]; then
  printf "expected dagu start <workflow.yaml>, got workflow: %s\n" "${1:-}" >&2
  exit 2
fi
yaml_path="$1"
cp "$yaml_path" "${WO_DAGU_CAPTURE_YAML:?}"
python3 - "$yaml_path" <<'PY' > "${WO_DAGU_COMMANDS_FILE:?}"
import re
import sys

path = sys.argv[1]
steps = []
current = None
reading_depends = False

def clean(value):
    value = value.strip()
    if len(value) >= 2 and value[0] == value[-1] == "'":
        return value[1:-1].replace("''", "'")
    if len(value) >= 2 and value[0] == value[-1] == '"':
        return value[1:-1].replace('\\"', '"')
    return value

with open(path, "r", encoding="utf-8") as fh:
    for raw in fh:
        line = raw.rstrip("\n")
        name_match = re.match(r"^\s*-\s+name:\s*(.+)$", line)
        if name_match:
            current = {"name": clean(name_match.group(1)), "command": "", "depends": []}
            steps.append(current)
            reading_depends = False
            continue
        if current is None:
            continue
        command_match = re.match(r"^\s+command:\s*(.+)$", line)
        if command_match:
            current["command"] = clean(command_match.group(1))
            reading_depends = False
            continue
        if re.match(r"^\s+depends:\s*$", line):
            reading_depends = True
            continue
        dep_match = re.match(r"^\s*-\s+(.+)$", line)
        if reading_depends and dep_match:
            current["depends"].append(clean(dep_match.group(1)))
            continue
        if re.match(r"^\s+\w+:", line):
            reading_depends = False

remaining = {step["name"]: step for step in steps}
done = set()
ordered = []
while remaining:
    progressed = False
    for name, step in list(remaining.items()):
        if all(dep in done for dep in step["depends"]):
            ordered.append(step)
            done.add(name)
            del remaining[name]
            progressed = True
    if not progressed:
        raise SystemExit("cyclic or missing Dagu dependencies: " + ",".join(sorted(remaining)))

for step in ordered:
    print(step["command"])
PY
while IFS= read -r command; do
  printf '%s\n' "$command" >> "${WO_DAGU_COMMAND_LOG:?}"
  bash -lc "$command" >> "${WO_DAGU_NODE_LOG:?}" 2>&1
done < "${WO_DAGU_COMMANDS_FILE:?}"
EOF
chmod +x "$FAKEBIN/dagu"

cat > "$FAKEBIN/codex" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
repo="$PWD"
while (($#)); do
  case "$1" in
    --cd) repo="$2"; shift 2 ;;
    *) shift ;;
  esac
done
prompt="$(cat)"
printf '%s\n---\n' "$prompt" >> "${WO_AGENT_PROMPTS:?}"
printf '{"type":"thread.started","thread_id":"thread-%s"}\n' "$(date +%s%N)"

if grep -q '^SUBAGENT_OUTPUT=' <<<"$prompt"; then
  output="$(awk -F= '/^SUBAGENT_OUTPUT=/{print $2; exit}' <<<"$prompt")"
  name="$(awk -F= '/^SUBAGENT_NAME=/{print $2; exit}' <<<"$prompt")"
  purpose="$(awk -F= '/^SUBAGENT_PURPOSE=/{print $2; exit}' <<<"$prompt")"
  mkdir -p "$(dirname "$output")"
  if [[ "$name" == "安全风险审核员" ]]; then
    python3 - "$output" "$name" "$purpose" <<'PY'
import json
import sys
path, name, purpose = sys.argv[1:4]
with open(path, "w", encoding="utf-8") as fh:
    json.dump({
        "name": name,
        "purpose": purpose,
        "status": "success",
        "summary": "发现 blocker 风险",
        "evidence": ["security blocker evidence"],
        "findings": [
            {
                "severity": "blocker",
                "title": "安全风险未处理",
                "evidence": "security blocker evidence",
                "recommendation": "修复后才能 clean"
            }
        ]
    }, fh, ensure_ascii=False)
PY
  else
    python3 - "$output" "$name" "$purpose" <<'PY'
import json
import sys
path, name, purpose = sys.argv[1:4]
with open(path, "w", encoding="utf-8") as fh:
    json.dump({
        "name": name,
        "purpose": purpose,
        "status": "success",
        "summary": name + " 完成",
        "evidence": [name + " evidence"],
        "findings": []
    }, fh, ensure_ascii=False)
PY
  fi
  exit 0
fi

if grep -q '^execution ' <<<"$prompt"; then
  printf -- '- [x] implement demo\n' > "$repo/docs/changes/demo/task.md"
elif grep -q '^review ' <<<"$prompt"; then
  review="$(awk '/^review /{print $2; exit}' <<<"$prompt")"
  parallel="$(awk '/^review /{print $3; exit}' <<<"$prompt")"
  test -s "$parallel"
  mkdir -p "$(dirname "$review")"
  cat > "$review" <<'JSON'
{
  "summary": "review incorrectly ignored blocker",
  "decision": "clean",
  "checks": {
    "oz_aligned": true,
    "tasks_verified": true,
    "tests_meaningful": true,
    "implementation_scoped": true,
    "runtime_behavior_verified": true,
    "previous_findings_resolved": true
  },
  "findings": [],
  "evidence": ["validation artifact was read", "runtime QA evidence reviewed"],
  "workflow_failure": null
}
JSON
elif grep -q '^qa ' <<<"$prompt"; then
  qa="$(awk '/^qa /{print $2; exit}' <<<"$prompt")"
  mkdir -p "$(dirname "$qa")"
  printf '{"summary":"should not run","decision":"clean","findings":[],"evidence":["unexpected"],"acceptance_matrix":[]}\n' > "$qa"
elif grep -q '^archive ' <<<"$prompt"; then
  delivery="$(awk '/^archive /{print $2; exit}' <<<"$prompt")"
  mkdir -p "$(dirname "$delivery")"
  printf 'should not archive\n' > "$delivery"
  mkdir -p "$repo/docs/changes/archive/2026-06-08-demo"
fi
EOF
chmod +x "$FAKEBIN/codex"

cat > wo.yaml <<'YAML'
wo:
  workflow:
    max_review_iterations: 1
    stages:
      execution:
        cli: codex
      review:
        cli: codex
      qa:
        cli: codex
      fix:
        cli: codex
      archive:
        cli: codex
    parallel:
      enabled: true
      groups:
        implementation_context:
          mode: advisory
          members:
            - name: 代码库侦察员
              purpose: 汇总 execution 需要读取的文件和测试模式
              tool: codex
        review:
          mode: gate_input
          members:
            - name: 目标核对审核员
              purpose: 核对 proposal/spec/task 是否满足
              tool: codex
            - name: 安全风险审核员
              purpose: 检查权限、输入和泄漏风险
              tool: codex
        qa:
          mode: gate_input
          members:
            - name: CLI/API 测试员
              purpose: 执行命令行真实路径
              tool: codex
    validation:
      commands: []
  prompts:
    execution: "execution {{.ParallelContextPath}}\n"
    review: "review {{.ReviewPath}} {{.ParallelReviewPath}}\n"
    qa: "qa {{.QAPath}} {{.ParallelQAPath}}\n"
    fix: "fix {{.FixSummaryPath}}\n"
    archive: "archive {{.DeliverySummaryPath}}\n"
YAML

export PATH="$FAKEBIN:/usr/bin:/bin"
export HOME="$HOME_DIR"
export XDG_STATE_HOME="$STATE_HOME"
export WO_DAGU_LOG="$RESULT_DIR/dagu.log"
export WO_DAGU_COMMAND_LOG="$RESULT_DIR/dagu-commands.log"
export WO_DAGU_COMMANDS_FILE="$TMP/dagu-commands.txt"
export WO_DAGU_CAPTURE_YAML="$RESULT_DIR/workflow.yaml"
export WO_DAGU_STDIN_YAML="$TMP/stdin-workflow.yaml"
export WO_DAGU_NODE_LOG="$RESULT_DIR/dagu-node-output.log"
export WO_AGENT_PROMPTS="$RESULT_DIR/agent-prompts.log"

set +e
"$BIN" run --change demo --engine dagu --json > "$RESULT_DIR/run.json" 2> "$RESULT_DIR/run.err"
status=$?
set -e
if [[ "$status" -eq 0 ]]; then
  echo "wo run --engine dagu succeeded despite blocker subagent finding" >&2
  exit 1
fi
if [[ ! -s "$RESULT_DIR/dagu.log" ]]; then
  echo "target behavior missing: wo run --engine dagu did not call the Dagu process before failing" >&2
  exit 1
fi

run_id="$(python3 - "$RESULT_DIR/run.json" <<'PY'
import json
import sys

records = []
with open(sys.argv[1], "r", encoding="utf-8") as fh:
    for raw in fh:
        raw = raw.strip()
        if raw:
            records.append(json.loads(raw))
if not records:
    raise SystemExit("wo run did not write runner JSON")
last = records[-1]
status = last.get("status")
if status not in {"failed", "blocked_review_limit"}:
    raise SystemExit(f"final runner JSON status = {status!r}, want failed or blocked_review_limit at review gate")
error = last.get("error", "")
for text in ("parallel-review-1", "clean review"):
    if text not in error:
        raise SystemExit(f"runner JSON error missing {text!r}: {error!r}")
run_id = last.get("run_id")
if not run_id:
    raise SystemExit("final runner JSON missing run_id")
print(run_id)
PY
)"
state="$(find "$STATE_HOME/wo/repos" -path "*/runs/$run_id/state.json" -print)"
test -s "$state"
run_dir="$(dirname "$state")"
cp "$state" "$RESULT_DIR/state.json"
test -s "$run_dir/dagu/workflow.yaml"
cmp "$run_dir/dagu/workflow.yaml" "$RESULT_DIR/workflow.yaml"
if [[ ! -s "$run_dir/parallel-review-1.json" ]]; then
  echo "target behavior missing: wo node fanin did not create parallel-review-1.json" >&2
  exit 1
fi
cp "$run_dir/parallel-review-1.json" "$RESULT_DIR/parallel-review-1.json"

grep -q '"severity": "blocker"' "$RESULT_DIR/parallel-review-1.json"
python3 - "$RESULT_DIR/state.json" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as fh:
    state = json.load(fh)
if state.get("status") not in {"failed", "blocked_review_limit"}:
    raise SystemExit(f"state status = {state.get('status')!r}, want failed/blocked_review_limit")
error = state.get("error", "")
for text in ("parallel-review-1", "clean review"):
    if text not in error:
        raise SystemExit(f"state error missing {text!r}: {error!r}")
if "validation" in error.lower():
    raise SystemExit(f"ignored blocker must fail at review gate, not validation: {error!r}")
PY
grep -q 'wo node gate' "$RESULT_DIR/dagu-commands.log"
grep -q -- '--stage review_1' "$RESULT_DIR/dagu-commands.log"
! grep -q -- '--stage qa_1' "$RESULT_DIR/dagu-commands.log"
! grep -q -- '--stage archive' "$RESULT_DIR/dagu-commands.log"
! grep -q '^qa ' "$RESULT_DIR/agent-prompts.log"
! grep -q '^archive ' "$RESULT_DIR/agent-prompts.log"
test ! -e "$run_dir/delivery-summary.md"
test ! -d docs/changes/archive/2026-06-08-demo

echo "contract passed: Dagu engine blocked ignored subagent finding"
