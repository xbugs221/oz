#!/usr/bin/env bash
# 文件功能目的：验证 subagent 节点即使写出 artifact，也不能在只读阶段修改源码。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/31-dagu-engine/subagent-readonly"
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
git init -q
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
  status) printf '{"change":"demo","status":"incomplete","tasks":{"total":1,"done":0}}\n' ;;
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
  bash -lc "$command"
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
  python3 - "$output" "$name" "$purpose" <<'PY'
import json
import sys
path, name, purpose = sys.argv[1:4]
with open(path, "w", encoding="utf-8") as fh:
    json.dump({
        "name": name,
        "purpose": purpose,
        "status": "success",
        "summary": "写出 artifact 但随后错误修改源码",
        "evidence": ["subagent artifact exists"]
    }, fh, ensure_ascii=False)
PY
  printf 'subagent changed source\n' >> "$repo/README.md"
fi
EOF
chmod +x "$FAKEBIN/codex"

cat > wo.yaml <<'YAML'
wo:
  workflow:
    max_review_iterations: 0
    stages:
      execution:
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
    validation:
      commands: []
  prompts:
    execution: "execution {{.ParallelContextPath}}\n"
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
export WO_AGENT_PROMPTS="$RESULT_DIR/agent-prompts.log"

set +e
"$BIN" run --change demo --engine dagu --json > "$RESULT_DIR/run.json" 2> "$RESULT_DIR/run.err"
status=$?
set -e
if [[ "$status" -eq 0 ]]; then
  echo "wo run --engine dagu succeeded even though a subagent modified source" >&2
  exit 1
fi
if [[ ! -s "$RESULT_DIR/dagu.log" ]]; then
  echo "target behavior missing: wo run --engine dagu did not call Dagu before failing" >&2
  exit 1
fi

state="$(find "$STATE_HOME/wo/repos" -path '*/runs/*/state.json' -print | sort | tail -n 1)"
test -s "$state"
cp "$state" "$RESULT_DIR/state.json"
grep -q 'wo node run-subagent' "$RESULT_DIR/dagu-commands.log"
grep -Eq '"status": "(failed|blocked_review_limit|blocked_validation_limit|aborted_manual_intervention)"' "$RESULT_DIR/state.json"
grep -Eiq 'subagent|只读|源码|手工|manual|worktree|git' "$RESULT_DIR/state.json" "$RESULT_DIR/run.err" "$RESULT_DIR/run.json"
test ! -d docs/changes/archive/2026-06-08-demo

echo "contract passed: Dagu engine rejected subagent source changes"
