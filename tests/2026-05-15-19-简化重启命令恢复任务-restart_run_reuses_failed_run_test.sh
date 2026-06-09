#!/usr/bin/env bash
# 验证单 run 因临时 agent 错误失败后，wo restart --run-id 复用同一个 run 并完成。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

bin="$tmp/wo"
go build -o "$bin" "$repo_root/cmd/wo"

work="$tmp/repo"
state_home="$tmp/state"
fakebin="$tmp/fakebin"
mkdir -p "$work/docs/changes/1-a" "$work/.wo/cmd" "$state_home" "$fakebin"
cd "$work"
git init >/dev/null
git config user.email test@example.com
git config user.name Test
printf 'demo\n' > README.md
git add README.md
git commit -m init >/dev/null
printf -- '- [ ] task\n' > docs/changes/1-a/task.md
cat > docs/changes/1-a/acceptance.json <<'JSON'
{"summary":"test acceptance","required_tests":[{"id":"contract-demo","source":"change_contract","path":"docs/changes/1-a/tests/demo.acceptance.test.ts","command":"true","purpose":"cover demo contract"}],"required_evidence":[{"id":"screenshot-demo","kind":"screenshot","path":"test-results/demo.png","purpose":"prove demo runtime"}]}
JSON
cat > wo.yaml <<'YAML'
wo:
  workflow:
    max_review_iterations: 0
  prompts:
    acceptance: "{{.Stage}} {{.AcceptancePath}}\n"
    execution: "{{.Stage}}\n"
    archive: "{{.Stage}} {{.DeliverySummaryPath}}\n"
YAML
cat > .wo/cmd/wo-start.md <<'EOF'
{{.Stage}}
EOF
cat > .wo/cmd/wo-done.md <<'EOF'
{{.Stage}} {{.DeliverySummaryPath}}
EOF

cat > "$fakebin/oz" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  list) printf '{"changes":[{"name":"1-a"}]}\n' ;;
  validate) printf '{"valid":true,"errors":[],"warnings":[],"artifacts":{}}\n' ;;
  status)
    if grep -q '\[x\]' "docs/changes/$2/task.md"; then
      printf '{"change":"%s","status":"ready","tasks":{"total":1,"done":1}}\n' "$2"
    else
      printf '{"change":"%s","status":"incomplete","tasks":{"total":1,"done":0}}\n' "$2"
    fi
    ;;
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
state="$(find "${XDG_STATE_HOME:?}/wo/repos" -path '*/runs/*/state.json' -print | sort | tail -n 1)"
change="$(grep -o '"change_name": "[^"]*"' "$state" | cut -d '"' -f 4)"
printf '{"type":"thread.started","thread_id":"fake-thread"}\n'
if [[ ! -e "$XDG_STATE_HOME/failed-once" ]]; then
  touch "$XDG_STATE_HOME/failed-once"
  exit 7
fi
case "$prompt" in
  acceptance*)
    acceptance="$(awk '{for (i=1; i<=NF; i++) if ($i ~ /acceptance\.json$/) print $i}' <<<"$prompt" | tail -n 1)"
    printf '%s\n' '{"summary":"test acceptance","required_tests":[{"id":"contract-demo","source":"change_contract","path":"docs/changes/demo/tests/demo.acceptance.test.ts","command":"true","purpose":"cover demo contract"}],"required_evidence":[{"id":"screenshot-demo","kind":"screenshot","path":"test-results/demo.png","purpose":"prove demo runtime"}]} ' > "$acceptance"
    ;;
  execution*) printf -- '- [x] task\n' > "$repo/docs/changes/$change/task.md" ;;
  archive*)
    mkdir -p "$repo/docs/changes/archive/2026-05-15-$change"
    delivery="$(awk '{for (i=1; i<=NF; i++) if ($i ~ /delivery-summary\.md$/) print $i}' <<<"$prompt" | tail -n 1)"
    printf 'done\n' > "$delivery"
    ;;
esac
EOF
chmod +x "$fakebin/codex"

set +e
PATH="$fakebin:/usr/bin:/bin" XDG_STATE_HOME="$state_home" "$bin" run --change 1-a --json > first.jsonl 2> first.err
first_code=$?
set -e
test "$first_code" -ne 0
run_id="$(python3 -c 'import json; print(json.loads(open("first.jsonl").readline())["run_id"])')"
grep -q '"status": "failed"' "$(find "$state_home/wo/repos" -path "*/runs/$run_id/state.json" -print -quit)"

if ! PATH="$fakebin:/usr/bin:/bin" XDG_STATE_HOME="$state_home" "$bin" restart --run-id "$run_id" --json > restart.jsonl 2> restart.err; then
  cat restart.jsonl
  cat restart.err >&2
  exit 1
fi
python3 -c 'import json; assert json.loads(open("restart.jsonl").readline())["status"] == "running"'
grep -q '"status": "done"' "$(find "$state_home/wo/repos" -path "*/runs/$run_id/state.json" -print -quit)"
test "$(find "$state_home/wo/repos" -path '*/runs/*/state.json' | wc -l)" -eq 1
