#!/usr/bin/env bash
# Build wo and trigger a real validation failure/retry flow in a temporary repository.
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

bin="$tmp/wo"
go build -o "$bin" "$repo_root/cmd/wo"

work="$tmp/work"
home="$tmp/home"
state_home="$tmp/state"
fakebin="$tmp/fakebin"
mkdir -p "$work" "$home" "$state_home" "$fakebin"
cd "$work"
git init >/dev/null
git config user.email test@example.com
git config user.name Test
printf 'demo\n' > README.md
mkdir -p docs/changes/demo
cat > docs/changes/demo/task.md <<'EOF'
- [ ] task
EOF
cat > docs/changes/demo/acceptance.json <<'JSON'
{"summary":"test acceptance","coverage":[{"spec":"validation gate retry","tests":["contract-demo"],"evidence":["screenshot-demo"],"risk":"validation fixture uses fake runtime evidence"}],"required_tests":[{"id":"contract-demo","source":"change_contract","path":"docs/changes/demo/tests/demo.acceptance.test.ts","command":"true","purpose":"cover demo contract","assertions":["execution reruns after validation failure, completes after validation-ok exists, and produces screenshot-demo evidence"]}],"required_evidence":[{"id":"screenshot-demo","kind":"screenshot","path":"test-results/demo.png","purpose":"prove demo runtime"}]}
JSON
git add .
git commit -m init >/dev/null

cat > "$fakebin/oz" <<'EOF'
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
chmod +x "$fakebin/oz"

cat > "$fakebin/codex" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
repo=""
while (($#)); do
  case "$1" in
    --cd) repo="$2"; shift 2 ;;
    *) shift ;;
  esac
done
prompt="$(cat)"
count_file="$repo/.fake-codex-count"
count=0
if [[ -f "$count_file" ]]; then
  count="$(cat "$count_file")"
fi
count=$((count + 1))
printf '%s' "$count" > "$count_file"
printf '%s\n' "$prompt" > "$repo/prompt-$count.txt"
state="$(find "${XDG_STATE_HOME:?}/wo/repos" -path '*/runs/*/state.json' -print | sort | tail -n 1)"
run_id="$(basename "$(dirname "$state")")"
printf '{"type":"thread.started","thread_id":"fake-thread-%s"}\n' "$count"
case "$prompt" in
  acceptance*)
    acceptance="$(awk '{for (i=1; i<=NF; i++) if ($i ~ /acceptance\.json$/) print $i}' <<<"$prompt" | tail -n 1)"
    printf '%s\n' '{"summary":"test acceptance","coverage":[{"spec":"validation gate retry","tests":["contract-demo"],"evidence":["screenshot-demo"],"risk":"validation fixture uses fake runtime evidence"}],"required_tests":[{"id":"contract-demo","source":"change_contract","path":"docs/changes/demo/tests/demo.acceptance.test.ts","command":"true","purpose":"cover demo contract","assertions":["execution reruns after validation failure, completes after validation-ok exists, and produces screenshot-demo evidence"]}],"required_evidence":[{"id":"screenshot-demo","kind":"screenshot","path":"test-results/demo.png","purpose":"prove demo runtime"}]} ' > "$acceptance"
    ;;
  execution*)
    execution_count_file="$repo/.fake-execution-count"
    execution_count=0
    if [[ -f "$execution_count_file" ]]; then
      execution_count="$(cat "$execution_count_file")"
    fi
    execution_count=$((execution_count + 1))
    printf '%s' "$execution_count" > "$execution_count_file"
    printf -- '- [x] task\n' > "$repo/docs/changes/demo/task.md"
    if [[ "$execution_count" -ge 2 ]]; then
      printf 'ok\n' > "$repo/validation-ok"
    fi
    ;;
  archive*)
    delivery="$(awk '{for (i=1; i<=NF; i++) if ($i ~ /delivery-summary\.md$/) print $i}' <<<"$prompt" | tail -n 1)"
    printf 'done\n' > "$delivery"
    mkdir -p "$repo/docs/changes/archive/2026-05-05-demo"
    ;;
esac
EOF
chmod +x "$fakebin/codex"
ln -sf "$fakebin/codex" "$fakebin/pi"
ln -sf "$fakebin/codex" "$fakebin/agy"

cat > wo.yaml <<'EOF'
max_review_iterations: 0
parallel: false
validation:
  limit: 2
  commands:
    - executable: test
      args: ["-f", "validation-ok"]
prompts:
  execution: "{{.Stage}}\n"
  archive: "{{.Stage}} {{.DeliverySummaryPath}}\n"
EOF

PATH="$fakebin:/usr/bin:/bin" HOME="$home" XDG_STATE_HOME="$state_home" "$bin" run --change demo --json > run.json
run_id="$(grep -o '"run_id":"[^"]*"' run.json | head -n 1 | cut -d '"' -f 4)"
test -n "$run_id"
artifact="$(find "$state_home/wo/repos" -path "*/runs/$run_id/validation-execution-1.json" -print -quit)"
test -s "$artifact"
grep -q '"status": "failed"' "$artifact"
grep -q '"command": "test -f validation-ok"' "$artifact"
grep -q '"exit_code": 1' "$artifact"
grep -q 'Validation gate failed' prompt-2.txt
grep -q "$artifact" prompt-2.txt
state="$(find "$state_home/wo/repos" -path "*/runs/$run_id/state.json" -print -quit)"
grep -q '"status": "done"' "$state"
test ! -e "$repo_root/.wo/runs/$run_id"
