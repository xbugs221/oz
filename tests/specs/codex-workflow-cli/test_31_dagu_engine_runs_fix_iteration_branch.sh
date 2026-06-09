#!/usr/bin/env bash
# 文件功能目的：验证 Dagu engine 在静态 DAG 中用 branch token 执行 needs_fix 循环，并跳过未激活分支。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/31-dagu-engine/fix-iteration"
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
cat > wo.yaml <<'YAML'
wo:
  workflow:
    max_review_iterations: 2
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
    validation:
      commands: []
  prompts:
    execution: "execution {{.ChangePath}}\n"
    review: "review {{.ReviewPath}}\n"
    qa: "qa {{.QAPath}}\n"
    fix: "fix {{.FixSummaryPath}}\n"
    archive: "archive {{.DeliverySummaryPath}}\n"
YAML
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
    if not step["command"]:
        raise SystemExit("missing command for step " + step["name"])
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

if grep -q '^execution ' <<<"$prompt"; then
  printf -- '- [x] implement demo\n' > "$repo/docs/changes/demo/task.md"
elif grep -q '^review ' <<<"$prompt"; then
  review="$(awk '/^review /{print $2; exit}' <<<"$prompt")"
  mkdir -p "$(dirname "$review")"
  case "$(basename "$review")" in
    review-1.json)
      cat > "$review" <<'JSON'
{
  "summary": "first review requires a fix",
  "decision": "needs_fix",
  "findings": [
    {
      "title": "README still needs the fix marker",
      "severity": "major",
      "evidence": "README.md lacks fixed marker",
      "recommendation": "run fix_1 before QA"
    }
  ],
  "evidence": ["review found missing fix marker"],
  "workflow_failure": null
}
JSON
      ;;
    review-2.json)
      cat > "$review" <<'JSON'
{
  "summary": "second review clean",
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
  "evidence": ["validation command passed", "runtime QA evidence reviewed"],
  "workflow_failure": null
}
JSON
      ;;
    *) echo "unexpected review path: $review" >&2; exit 41 ;;
  esac
elif grep -q '^fix ' <<<"$prompt"; then
  fix="$(awk '/^fix /{print $2; exit}' <<<"$prompt")"
  mkdir -p "$(dirname "$fix")"
  case "$(basename "$fix")" in
    fix-1-summary.md)
      printf 'applied first-round fix\n' > "$fix"
      printf 'fixed\n' >> "$repo/README.md"
      ;;
    *) echo "inactive fix branch called agent: $fix" >&2; exit 42 ;;
  esac
elif grep -q '^qa ' <<<"$prompt"; then
  qa="$(awk '/^qa /{print $2; exit}' <<<"$prompt")"
  mkdir -p "$(dirname "$qa")"
  case "$(basename "$qa")" in
    qa-2.json)
      cat > "$qa" <<'JSON'
{
  "summary": "qa clean after fix",
  "decision": "clean",
  "findings": [],
  "evidence": ["runtime QA path executed after fix"],
  "acceptance_matrix": [
    {
      "id": "contract-demo",
      "status": "passed",
      "evidence": "true command covered demo contract"
    }
  ]
}
JSON
      ;;
    *) echo "inactive QA branch called agent: $qa" >&2; exit 43 ;;
  esac
elif grep -q '^archive ' <<<"$prompt"; then
  delivery="$(awk '/^archive /{print $2; exit}' <<<"$prompt")"
  mkdir -p "$(dirname "$delivery")"
  printf 'fix iteration delivery summary\n' > "$delivery"
  mkdir -p "$repo/docs/changes/archive/2026-06-08-demo"
fi
EOF
chmod +x "$FAKEBIN/codex"

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
if [[ ! -s "$RESULT_DIR/dagu.log" ]]; then
  echo "target behavior missing: wo run --engine dagu did not call the Dagu process" >&2
  exit 1
fi
if [[ "$status" -ne 0 ]]; then
  exit "$status"
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
if last.get("status") != "done":
    raise SystemExit(f"final runner JSON status = {last.get('status')!r}, want done")
print(last["run_id"])
PY
)"
state="$(find "$STATE_HOME/wo/repos" -path "*/runs/$run_id/state.json" -print)"
test -s "$state"
run_dir="$(dirname "$state")"
cp "$state" "$RESULT_DIR/state.json"

grep -q '"status": "done"' "$RESULT_DIR/state.json"
test -s "$run_dir/review-1.json"
test -s "$run_dir/fix-1-summary.md"
test -s "$run_dir/review-2.json"
test -s "$run_dir/qa-2.json"
test -s "$run_dir/delivery-summary.md"
test ! -e "$run_dir/qa-1.json"
test ! -e "$run_dir/fix-2-summary.md"

grep -q -- '--stage qa_1' "$RESULT_DIR/dagu-commands.log"
grep -q -- '--stage fix_2' "$RESULT_DIR/dagu-commands.log"
grep -q '"status": "skipped"' "$RESULT_DIR/dagu-node-output.log"
grep -q '^fix ' "$RESULT_DIR/agent-prompts.log"
grep -q 'fix-1-summary.md' "$RESULT_DIR/agent-prompts.log"
grep -q 'review-2.json' "$RESULT_DIR/agent-prompts.log"
grep -q 'qa-2.json' "$RESULT_DIR/agent-prompts.log"
if grep -q 'qa-1.json' "$RESULT_DIR/agent-prompts.log"; then
  echo "inactive QA iteration called the agent instead of skipping" >&2
  exit 1
fi
if grep -q 'fix-2-summary.md' "$RESULT_DIR/agent-prompts.log"; then
  echo "inactive fix iteration called the agent instead of skipping" >&2
  exit 1
fi

echo "contract passed: Dagu engine handled needs_fix iteration branching"
