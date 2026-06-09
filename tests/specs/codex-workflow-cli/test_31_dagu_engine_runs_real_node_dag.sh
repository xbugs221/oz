#!/usr/bin/env bash
# 文件功能目的：验证 Dagu engine 真正调用 Dagu 外部进程，并通过 wo node 执行 OmO fan-out/fan-in、主阶段、gate 和归档。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/31-dagu-engine/success"
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
  "required_evidence": [
    {
      "id": "runtime-demo",
      "kind": "runtime_log",
      "path": "test-results/demo/runtime.log",
      "purpose": "prove demo runtime"
    }
  ]
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
printf 'dagu args: %s\n' "$*"
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
  printf 'dagu command: %s\n' "$command"
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
session="thread-$(date +%s%N)"
printf '{"type":"thread.started","thread_id":"%s"}\n' "$session"

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
        "summary": name + " 完成",
        "evidence": [name + " evidence"],
        "findings": []
    }, fh, ensure_ascii=False)
PY
  exit 0
fi

if grep -q '^execution ' <<<"$prompt"; then
  parallel="$(awk '/^execution /{print $2; exit}' <<<"$prompt")"
  test -s "$parallel"
  printf -- '- [x] implement demo\n' > "$repo/docs/changes/demo/task.md"
elif grep -q '^review ' <<<"$prompt"; then
  review="$(awk '/^review /{print $2; exit}' <<<"$prompt")"
  parallel="$(awk '/^review /{print $3; exit}' <<<"$prompt")"
  test -s "$parallel"
  mkdir -p "$(dirname "$review")"
  cat > "$review" <<'JSON'
{
  "summary": "parallel review clean",
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
  parallel="$(awk '/^qa /{print $3; exit}' <<<"$prompt")"
  test -s "$parallel"
  mkdir -p "$(dirname "$qa")"
  cat > "$qa" <<'JSON'
{
  "summary": "parallel QA clean",
  "decision": "clean",
  "findings": [],
  "evidence": ["parallel QA artifact was read"],
  "acceptance_matrix": [
    {
      "id": "contract-demo",
      "status": "passed",
      "evidence": "true command covered demo contract"
    },
    {
      "id": "runtime-demo",
      "status": "passed",
      "evidence": "runtime log captured"
    }
  ]
}
JSON
elif grep -q '^archive ' <<<"$prompt"; then
  delivery="$(awk '/^archive /{print $2; exit}' <<<"$prompt")"
  mkdir -p "$(dirname "$delivery")"
  printf 'Dagu engine delivery summary\n' > "$delivery"
  mkdir -p "$repo/docs/changes/archive/2026-06-08-demo"
elif grep -q '^fix ' <<<"$prompt"; then
  printf 'fix should have been skipped after clean review and clean QA\n' > "${WO_FIX_CALLED_MARKER:?}"
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
            - name: 外部资料研究员
              purpose: 查询 execution 依赖的外部库文档
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
            - name: 回归场景测试员
              purpose: 覆盖邻近功能回归
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
export WO_FIX_CALLED_MARKER="$RESULT_DIR/fix-called"

set +e
"$BIN" run --change demo --engine dagu --json > "$RESULT_DIR/run.json" 2> "$RESULT_DIR/run.err"
status=$?
set -e
if [[ "$status" -ne 0 ]]; then
  if [[ ! -s "$RESULT_DIR/dagu.log" ]]; then
    echo "target behavior missing: wo run --engine dagu did not call the Dagu process" >&2
  fi
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

grep -q '"status": "done"' "$RESULT_DIR/state.json"
test -s "$run_dir/prompt-snapshot.yaml"
test -s "$run_dir/acceptance.json"
run_log="$(find "$run_dir" -type f -name '*.log' -print | sort | head -n 1)"
test -s "$run_log"
cp "$run_log" "$RESULT_DIR/run-dir-dagu.log"
grep -Eq 'dagu|wo node' "$RESULT_DIR/run-dir-dagu.log"
test -s "$RESULT_DIR/workflow.yaml"
test -s "$run_dir/dagu/workflow.yaml"
cp "$run_dir/dagu/workflow.yaml" "$RESULT_DIR/run-dir-workflow.yaml"
cmp "$RESULT_DIR/run-dir-workflow.yaml" "$RESULT_DIR/workflow.yaml"
python3 - "$RESULT_DIR/run-dir-workflow.yaml" "$run_id" <<'PY'
import re
import sys

path, run_id = sys.argv[1:3]
allowed = (
    "wo node run-subagent ",
    "wo node fanin ",
    "wo node run-stage ",
    "wo node gate ",
)
commands = []
with open(path, "r", encoding="utf-8") as fh:
    for raw in fh:
        match = re.match(r"^\s+command:\s*(.+)$", raw.rstrip("\n"))
        if not match:
            continue
        command = match.group(1).strip()
        if len(command) >= 2 and command[0] == command[-1] == "'":
            command = command[1:-1].replace("''", "'")
        commands.append(command)

if not commands:
    raise SystemExit("run-specific Dagu YAML has no step commands")
for command in commands:
    if not command.startswith(allowed):
        raise SystemExit(f"Dagu command is not a wo node command: {command}")
    run_id_forms = (f"--run-id {run_id}", f"--run-id '{run_id}'", f'--run-id "{run_id}"')
    if not any(form in command for form in run_id_forms):
        raise SystemExit(f"Dagu command missing matching --run-id: {command}")
    for forbidden in ("codex exec", "opencode run", "pi --mode json"):
        if forbidden in command:
            raise SystemExit(f"Dagu command directly calls backend CLI: {command}")
PY

for text in 'wo node run-subagent' 'wo node fanin' 'wo node run-stage' 'wo node gate'; do
  grep -q "$text" "$RESULT_DIR/dagu-commands.log"
done
grep -q -- '--stage fix_1' "$RESULT_DIR/dagu-commands.log"
grep -q '"status": "skipped"' "$RESULT_DIR/dagu-node-output.log"
test ! -e "$RESULT_DIR/fix-called"

for artifact in \
  parallel-implementation-context.json \
  parallel-review-1.json \
  parallel-qa-1.json \
  review-1.json \
  qa-1.json \
  delivery-summary.md; do
  test -s "$run_dir/$artifact"
done

for token in SUBAGENT_GROUP= SUBAGENT_NAME= SUBAGENT_PURPOSE= SUBAGENT_OUTPUT=; do
  grep -q "$token" "$RESULT_DIR/agent-prompts.log"
done
grep -q '代码库侦察员' "$run_dir/parallel-implementation-context.json"
grep -q '目标核对审核员' "$run_dir/parallel-review-1.json"
grep -q 'CLI/API 测试员' "$run_dir/parallel-qa-1.json"

python3 - "$run_dir/parallel-implementation-context.json" "$run_dir/parallel-review-1.json" "$run_dir/parallel-qa-1.json" <<'PY'
import json
import sys

expected = [
    ("implementation_context", "advisory", {"代码库侦察员", "外部资料研究员"}),
    ("review", "gate_input", {"目标核对审核员", "安全风险审核员"}),
    ("qa", "gate_input", {"CLI/API 测试员", "回归场景测试员"}),
]
for path, (group, mode, members) in zip(sys.argv[1:], expected):
    with open(path, "r", encoding="utf-8") as fh:
        artifact = json.load(fh)
    if artifact.get("group") != group:
        raise SystemExit(f"{path} group = {artifact.get('group')!r}, want {group!r}")
    if artifact.get("mode") != mode:
        raise SystemExit(f"{path} mode = {artifact.get('mode')!r}, want {mode!r}")
    actual = [member.get("name") for member in artifact.get("members", [])]
    if set(actual) != members or len(actual) != len(set(actual)):
        raise SystemExit(f"{path} members = {actual!r}, want exact {sorted(members)!r}")
    for member in artifact.get("members", []):
        for key in ("name", "purpose", "status", "summary", "evidence"):
            if key not in member:
                raise SystemExit(f"{path} member {member.get('name')!r} missing {key}")
        if "findings" in member and not isinstance(member["findings"], list):
            raise SystemExit(f"{path} member {member.get('name')!r} findings is not a list")
PY

echo "contract passed: Dagu engine executed real wo node DAG"
