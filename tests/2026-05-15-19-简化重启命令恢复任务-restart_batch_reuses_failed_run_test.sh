#!/usr/bin/env bash
# 验证 batch 中首个 run 临时失败后，wo restart --batch-id 复用原 run 并继续后续队列。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

bin="$tmp/wo"
go build -o "$bin" "$repo_root/cmd/wo"

work="$tmp/repo"
state_home="$tmp/state"
fakebin="$tmp/fakebin"
mkdir -p "$work/docs/changes/1-a" "$work/docs/changes/2-b" "$work/.wo/cmd" "$state_home" "$fakebin"
cd "$work"
git init >/dev/null
git config user.email test@example.com
git config user.name Test
printf 'demo\n' > README.md
git add README.md
git commit -m init >/dev/null
printf -- '- [ ] task\n' > docs/changes/1-a/task.md
printf -- '- [ ] task\n' > docs/changes/2-b/task.md
for change in 1-a 2-b; do
  cat > "docs/changes/$change/acceptance.json" <<JSON
{"summary":"test acceptance","required_tests":[{"id":"contract-demo","source":"change_contract","path":"docs/changes/$change/tests/demo.acceptance.test.ts","command":"true","purpose":"cover demo contract","assertions":["temporary workflow fixture exercises the script-specific business path"]}],"required_evidence":[{"id":"screenshot-demo","kind":"screenshot","path":"test-results/demo.png","purpose":"prove demo runtime"}]}
JSON
done
cat > wo.yaml <<'YAML'
wo:
  workflow:
    max_review_iterations: 0
    parallel:
      enabled: false
  prompts:
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
  list) printf '{"changes":[{"name":"1-a"},{"name":"2-b"}]}\n' ;;
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
state=""
for candidate in "${XDG_STATE_HOME:?}/wo/repos"/*/runs/*/state.json; do
  [[ -f "$candidate" ]] || continue
  if grep -q '"status": "running"' "$candidate"; then state="$candidate"; fi
done
change="$(grep -o '"change_name": "[^"]*"' "$state" | cut -d '"' -f 4)"
printf '{"type":"thread.started","thread_id":"fake-thread"}\n'
if [[ "$change" == "1-a" && ! -e "$XDG_STATE_HOME/failed-once" ]]; then
  touch "$XDG_STATE_HOME/failed-once"
  exit 7
fi
case "$prompt" in
  acceptance*)
    acceptance="$(awk '{for (i=1; i<=NF; i++) if ($i ~ /acceptance\.json$/) print $i}' <<<"$prompt" | tail -n 1)"
    printf '%s\n' '{"summary":"test acceptance","required_tests":[{"id":"contract-demo","source":"change_contract","path":"docs/changes/demo/tests/demo.acceptance.test.ts","command":"true","purpose":"cover demo contract","assertions":["temporary workflow fixture exercises the script-specific business path"]}],"required_evidence":[{"id":"screenshot-demo","kind":"screenshot","path":"test-results/demo.png","purpose":"prove demo runtime"}]} ' > "$acceptance"
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
ln -sf "$fakebin/codex" "$fakebin/pi"

repo_key="$(python3 - <<'PY'
import hashlib, os
repo=os.getcwd()
print(os.path.basename(repo).lower() + "-" + hashlib.sha1(repo.encode()).hexdigest()[:10])
PY
)"
mkdir -p "$state_home/wo/repos/$repo_key/batches/batch-restart"
cat > "$state_home/wo/repos/$repo_key/batches/batch-restart/state.json" <<'JSON'
{"batch_id":"batch-restart","status":"running","changes":["1-a","2-b"],"current_index":0,"run_ids":{},"error":""}
JSON

set +e
PATH="$fakebin:/usr/bin:/bin" XDG_STATE_HOME="$state_home" "$bin" batch --batch-id batch-restart --json > first.out 2> first.err
first_code=$?
set -e
test "$first_code" -ne 0
batch_state="$state_home/wo/repos/$repo_key/batches/batch-restart/state.json"
first_run="$(python3 -c 'import json; print(json.load(open("'"$batch_state"'"))["failed_run_id"])')"
grep -q '"status": "failed"' "$batch_state"

PATH="$fakebin:/usr/bin:/bin" XDG_STATE_HOME="$state_home" "$bin" restart --batch-id batch-restart --json > restart.out
grep -q '"status": "done"' "$batch_state"
# New semantics: restart clears old run association, batch worker creates a new run.
# The new run ID should be different from the old failed run.
new_run="$(python3 -c 'import json; print(json.load(open("'"$batch_state"'"))["run_ids"]["1-a"])')"
test "$new_run" != "$first_run" || { echo "new run $new_run should differ from old failed run $first_run"; exit 1; }
test "$(python3 -c 'import json; print(json.load(open("'"$batch_state"'"))["current_index"])')" = "2"
