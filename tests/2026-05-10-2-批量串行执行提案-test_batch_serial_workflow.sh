#!/usr/bin/env bash
# Build wo and exercise batch selection, serial ordering, failure stop, and resume in temporary repositories.
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

bin="$tmp/wo"
go build -o "$bin" "$repo_root/cmd/wo"

make_repo() {
  local work="$1"
  mkdir -p "$work" "$work/docs/changes" "$work/.wo/cmd"
  cd "$work"
  git init >/dev/null
  git config user.email test@example.com
  git config user.name Test
  printf 'demo\n' > README.md
  git add README.md
  git commit -m init >/dev/null
cat > wo.yaml <<'EOF'
wo:
  workflow:
    max_review_iterations: 0
  prompts:
    acceptance: "{{.Stage}} {{.AcceptancePath}}\n"
    execution: "{{.Stage}}\n"
    archive: "{{.Stage}} {{.DeliverySummaryPath}}\n"
EOF
  cat > .wo/cmd/wo-start.md <<'EOF'
{{.Stage}}
EOF
  cat > .wo/cmd/wo-done.md <<'EOF'
{{.Stage}}
EOF
}

make_change() {
  local name="$1"
  mkdir -p "docs/changes/$name"
  printf -- '- [ ] task\n' > "docs/changes/$name/task.md"
  cat > "docs/changes/$name/acceptance.json" <<JSON
{"summary":"test acceptance","required_tests":[{"id":"contract-demo","source":"change_contract","path":"docs/changes/$name/tests/demo.acceptance.test.ts","command":"true","purpose":"cover demo contract"}],"required_evidence":[{"id":"screenshot-demo","kind":"screenshot","path":"test-results/demo.png","purpose":"prove demo runtime"}]}
JSON
}

make_fakebin() {
  local fakebin="$1"
  mkdir -p "$fakebin"
  cat > "$fakebin/oz" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  list)
    printf '{"changes":['
    first=1
    for dir in docs/changes/*; do
      [[ -d "$dir" ]] || continue
      name="$(basename "$dir")"
      [[ "$name" == archive ]] && continue
      if [[ "$first" -eq 0 ]]; then printf ','; fi
      first=0
      printf '{"name":"%s"}' "$name"
    done
    printf ']}\n'
    ;;
  validate) printf '{"valid":true,"errors":[],"warnings":[],"artifacts":{}}\n' ;;
  status)
    change="$2"
    if grep -q '\[x\]' "docs/changes/$change/task.md"; then
      printf '{"change":"%s","status":"ready","tasks":{"total":1,"done":1}}\n' "$change"
    else
      printf '{"change":"%s","status":"incomplete","tasks":{"total":1,"done":0}}\n' "$change"
    fi
    ;;
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
run_id=""
for state in "${XDG_STATE_HOME:?}/wo/repos"/*/runs/*/state.json; do
  [[ -f "$state" ]] || continue
  if grep -q '"status": "running"' "$state"; then
    run_id="$(basename "$(dirname "$state")")"
  fi
done
if [[ -z "$run_id" ]]; then
  run_id="$(basename "$(find "$XDG_STATE_HOME/wo/repos" -path '*/runs/*' -type d | sort | tail -n 1)")"
fi
state="$(find "$XDG_STATE_HOME/wo/repos" -path "*/runs/$run_id/state.json" -print -quit)"
change="$(grep -o '"change_name": "[^"]*"' "$state" | head -n 1 | cut -d '"' -f 4)"
printf '{"type":"thread.started","thread_id":"fake-thread"}\n'
if [[ "${WO_FAIL_CHANGE:-}" == "$change" ]]; then
  exit 7
fi
case "$prompt" in
  acceptance*)
    acceptance="$(awk '{for (i=1; i<=NF; i++) if ($i ~ /acceptance\.json$/) print $i}' <<<"$prompt" | tail -n 1)"
    printf '%s\n' '{"summary":"test acceptance","required_tests":[{"id":"contract-demo","source":"change_contract","path":"docs/changes/demo/tests/demo.acceptance.test.ts","command":"true","purpose":"cover demo contract"}],"required_evidence":[{"id":"screenshot-demo","kind":"screenshot","path":"test-results/demo.png","purpose":"prove demo runtime"}]} ' > "$acceptance"
    ;;
  execution*) printf -- '- [x] task\n' > "$repo/docs/changes/$change/task.md" ;;
  archive*)
    mkdir -p "$repo/docs/changes/archive/2026-05-10-$change"
    delivery="$(awk '{for (i=1; i<=NF; i++) if ($i ~ /delivery-summary\.md$/) print $i}' <<<"$prompt" | tail -n 1)"
    printf 'done\n' > "$delivery"
    ;;
esac
EOF
  chmod +x "$fakebin/codex"
}

fakebin="$tmp/fakebin"
make_fakebin "$fakebin"

work="$tmp/work-success"
state_home="$tmp/state-success"
mkdir -p "$state_home"
make_repo "$work"
make_change "5-c"
make_change "3-a"
make_change "4-b"
cat > .wo/cmd/wo-done.md <<'EOF'
{{.Stage}} {{.DeliverySummaryPath}}
EOF
printf '2\n3,1-2\n' | PATH="$fakebin:/usr/bin:/bin" XDG_STATE_HOME="$state_home" "$bin" >/dev/null
sleep 1
batch_state="$(find "$state_home/wo/repos" -path '*/batches/*/state.json' -print | sort | tail -n 1)"
grep -q '"changes": \[' "$batch_state"
grep -q '"3-a"' "$batch_state"
grep -q '"status": "done"' "$batch_state"
test "$(find "$state_home/wo/repos" -path '*/runs/*/state.json' | wc -l)" -eq 3
test ! -e .wo/runs
test ! -e .wo/batches

work="$tmp/work-failure"
state_home="$tmp/state-failure"
mkdir -p "$state_home"
make_repo "$work"
make_change "1-a"
make_change "2-b"
repo_key="$(basename "$PWD" | tr '[:upper:]' '[:lower:]')-$(printf '%s' "$PWD" | sha1sum | cut -c1-10)"
mkdir -p "$state_home/wo/repos/$repo_key/batches/batch-fail"
cat > "$state_home/wo/repos/$repo_key/batches/batch-fail/state.json" <<'EOF'
{"batch_id":"batch-fail","status":"running","changes":["1-a","2-b"],"current_index":0,"run_ids":{},"error":""}
EOF
set +e
PATH="$fakebin:/usr/bin:/bin" XDG_STATE_HOME="$state_home" WO_FAIL_CHANGE="1-a" "$bin" batch --batch-id batch-fail --json >/dev/null 2>fail.err
status=$?
set -e
test "$status" -ne 0
grep -q '"status": "failed"' "$state_home/wo/repos/$repo_key/batches/batch-fail/state.json"
test "$(grep -c '"change_name"' "$state_home"/wo/repos/*/runs/*/state.json)" -eq 1

work="$tmp/work-resume"
state_home="$tmp/state-resume"
mkdir -p "$state_home"
make_repo "$work"
make_change "1-a"
make_change "2-b"
repo_key="$(basename "$PWD" | tr '[:upper:]' '[:lower:]')-$(printf '%s' "$PWD" | sha1sum | cut -c1-10)"
mkdir -p "$state_home/wo/repos/$repo_key/runs/done-run"
cat > "$state_home/wo/repos/$repo_key/runs/done-run/state.json" <<'EOF'
{"run_id":"done-run","change_name":"1-a","status":"done","stage":"done","stages":{},"paths":{},"sessions":{},"error":"","workflow_config":{"max_review_iterations":0,"stages":{},"validation":{"max_attempts_per_stage":3}}}
EOF
mkdir -p "$state_home/wo/repos/$repo_key/batches/batch-resume"
cat > "$state_home/wo/repos/$repo_key/batches/batch-resume/state.json" <<'EOF'
{"batch_id":"batch-resume","status":"running","changes":["1-a","2-b"],"current_index":0,"run_ids":{"1-a":"done-run"},"error":""}
EOF
cat > .wo/cmd/wo-done.md <<'EOF'
{{.Stage}} {{.DeliverySummaryPath}}
EOF
PATH="$fakebin:/usr/bin:/bin" XDG_STATE_HOME="$state_home" "$bin" batch --batch-id batch-resume --json >/dev/null
grep -q '"status": "done"' "$state_home/wo/repos/$repo_key/batches/batch-resume/state.json"
grep -q '"2-b"' "$state_home/wo/repos/$repo_key/batches/batch-resume/state.json"
