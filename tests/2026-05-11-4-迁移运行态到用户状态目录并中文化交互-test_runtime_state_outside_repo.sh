#!/usr/bin/env bash
# Build wo and verify a foreground sealed run writes state and artifacts outside the repository.
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
git add README.md
git commit -m init >/dev/null
mkdir -p docs/changes/demo
cat > docs/changes/demo/task.md <<'EOF'
- [ ] implement demo
EOF
cat > docs/changes/demo/acceptance.json <<'JSON'
{"summary":"test acceptance","required_tests":[{"id":"contract-demo","source":"change_contract","path":"docs/changes/demo/tests/demo.acceptance.test.ts","command":"true","purpose":"cover demo contract","assertions":["temporary workflow fixture exercises the script-specific business path"]}],"required_evidence":[{"id":"screenshot-demo","kind":"screenshot","path":"test-results/demo.png","purpose":"prove demo runtime"}]}
JSON

cat > "$fakebin/oz" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  validate) printf '{"valid":true,"change":"%s","errors":[],"warnings":[],"artifacts":{}}\n' "$2" ;;
  status)
    if grep -q '\[x\]' "docs/changes/$2/task.md"; then
      printf '{"change":"%s","status":"complete","tasks":{"total":1,"done":1}}\n' "$2"
    else
      printf '{"change":"%s","status":"incomplete","tasks":{"total":1,"done":0}}\n' "$2"
    fi
    ;;
  list) printf '{"changes":[{"name":"demo"}]}\n' ;;
esac
EOF
chmod +x "$fakebin/oz"

cat > "$fakebin/codex" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
repo="$PWD"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --cd) repo="$2"; shift 2 ;;
    *) shift ;;
  esac
done
prompt="$(cat)"
printf '{"type":"thread.started","thread_id":"fake-thread"}\n'
if grep -q '^acceptance ' <<<"$prompt"; then
  path="$(awk '{for (i=1; i<=NF; i++) if ($i ~ /acceptance\.json$/) print $i}' <<<"$prompt" | tail -n 1)"
  mkdir -p "$(dirname "$path")"
  printf '%s\n' '{"summary":"test acceptance","required_tests":[{"id":"contract-demo","source":"change_contract","path":"docs/changes/demo/tests/demo.acceptance.test.ts","command":"true","purpose":"cover demo contract","assertions":["temporary workflow fixture exercises the script-specific business path"]}],"required_evidence":[{"id":"screenshot-demo","kind":"screenshot","path":"test-results/demo.png","purpose":"prove demo runtime"}]} ' > "$path"
elif grep -q '^execution$' <<<"$prompt"; then
  printf -- '- [x] implement demo\n' > "$repo/docs/changes/demo/task.md"
elif grep -q '^archive ' <<<"$prompt"; then
  path="$(awk '{for (i=1; i<=NF; i++) if ($i ~ /delivery-summary\.md$/) print $i}' <<<"$prompt" | tail -n 1)"
  mkdir -p "$(dirname "$path")"
  printf 'done\n' > "$path"
  mkdir -p "$repo/docs/changes/archive/2026-05-05-demo"
fi
EOF
chmod +x "$fakebin/codex"
ln -sf "$fakebin/codex" "$fakebin/pi"
ln -sf "$fakebin/codex" "$fakebin/agy"

cat > wo.yaml <<'EOF'
wo:
  workflow:
    max_review_iterations: 0
    parallel:
      enabled: false
  prompts:
    execution: "{{.Stage}}\n"
    archive: "{{.Stage}} {{.DeliverySummaryPath}}\n"
EOF

PATH="$fakebin:/usr/bin:/bin" HOME="$home" XDG_STATE_HOME="$state_home" "$bin" run --change demo --json > run.json
run_id="$(grep -o '"run_id":"[^"]*"' run.json | head -n 1 | cut -d '"' -f 4)"
test -n "$run_id"
test ! -e .wo/runs
state="$(find "$state_home/wo/repos" -path "*/runs/$run_id/state.json" -print -quit)"
test -s "$state"
delivery="$(dirname "$state")/delivery-summary.md"
test -s "$delivery"
grep -q '"status": "done"' "$state"
