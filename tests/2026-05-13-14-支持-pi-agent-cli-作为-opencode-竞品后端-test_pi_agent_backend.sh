#!/usr/bin/env bash
# Build wo and verify Pi agent backend behavior through public sealed-run commands.
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

bin="$tmp/wo"
go build -o "$bin" "$repo_root/cmd/wo"

make_repo() {
  local work="$1"
  mkdir -p "$work/docs/changes/demo"
  cd "$work"
  git init >/dev/null
  git config user.email test@example.com
  git config user.name Test
  printf 'demo\n' > README.md
  cat > docs/changes/demo/task.md <<'TASK'
- [ ] implement demo
TASK
  cat > docs/changes/demo/acceptance.json <<'JSON'
{"summary":"test acceptance","required_tests":[{"id":"contract-demo","source":"change_contract","path":"docs/changes/demo/tests/demo.acceptance.test.ts","command":"true","purpose":"cover demo contract","assertions":["temporary workflow fixture exercises the script-specific business path"]}],"required_evidence":[{"id":"screenshot-demo","kind":"screenshot","path":"test-results/demo.png","purpose":"prove demo runtime"}]}
JSON
  git add .
  git commit -m init >/dev/null
}

install_fake_oz() {
  local dir="$1"
  cat > "$dir/oz" <<'OZ'
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
  *) printf 'unexpected oz command: %s\n' "$*" >&2; exit 2 ;;
esac
OZ
  chmod +x "$dir/oz"
}

install_fake_pi() {
  local dir="$1"
  cat > "$dir/pi" <<'PI'
#!/usr/bin/env bash
set -euo pipefail
session=""
prompt=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode)
      test "$2" = "json"
      shift 2
      ;;
    --model)
      printf '%s\n' "$2" >> "$PWD/pi-models.log"
      shift 2
      ;;
    --thinking)
      printf '%s\n' "$2" >> "$PWD/pi-thinking.log"
      shift 2
      ;;
    --session)
      session="$2"
      printf '%s\n' "$2" >> "$PWD/pi-sessions.log"
      shift 2
      ;;
    *)
      prompt="$1"
      shift
      ;;
  esac
done
if grep -q 'fast_mode' <<<"$*"; then
  printf 'unexpected fast flag\n' >&2
  exit 9
fi
if [[ -z "$session" ]]; then
  session="pi-session-1"
fi
printf '{"type":"session","id":"%s"}\n' "$session"
if grep -q '^acceptance ' <<<"$prompt"; then
  path="$(awk '{for (i=1; i<=NF; i++) if ($i ~ /acceptance\.json$/) print $i}' <<<"$prompt" | tail -n 1)"
  mkdir -p "$(dirname "$path")"
  printf '%s\n' '{"summary":"test acceptance","required_tests":[{"id":"contract-demo","source":"change_contract","path":"docs/changes/demo/tests/demo.acceptance.test.ts","command":"true","purpose":"cover demo contract","assertions":["temporary workflow fixture exercises the script-specific business path"]}],"required_evidence":[{"id":"screenshot-demo","kind":"screenshot","path":"test-results/demo.png","purpose":"prove demo runtime"}]} ' > "$path"
elif grep -q '^execution$' <<<"$prompt"; then
  printf -- '- [x] implement demo\n' > docs/changes/demo/task.md
elif grep -q '^archive ' <<<"$prompt"; then
  path="$(awk '{for (i=1; i<=NF; i++) if ($i ~ /delivery-summary\.md$/) print $i}' <<<"$prompt" | tail -n 1)"
  mkdir -p "$(dirname "$path")"
  printf 'done\n' > "$path"
  mkdir -p docs/changes/archive/2026-05-05-demo
fi
PI
  chmod +x "$dir/pi"
}

install_fake_codex() {
  local dir="$1"
  cat > "$dir/codex" <<'CODEX'
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
printf '{"type":"thread.started","thread_id":"codex-session"}\n'
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
CODEX
  chmod +x "$dir/codex"
}

success_repo="$tmp/success"
state_home="$tmp/state-success"
fakebin="$tmp/fakebin-success"
success_home="$tmp/home-success"
mkdir -p "$fakebin" "$state_home" "$success_home"
make_repo "$success_repo"
install_fake_oz "$fakebin"
install_fake_codex "$fakebin"
install_fake_pi "$fakebin"
ln -sf "$fakebin/pi" "$fakebin/agy"
cat > wo.yaml <<'YAML'
wo:
  workflow:
    max_review_iterations: 0
    parallel:
      enabled: false
    stages:
      execution:
        cli: pi
        model: anthropic/claude-sonnet
        reasoning: high
        fast: true
      archive:
        cli: pi
        reasoning: low
  prompts:
    execution: |
      {{.Stage}}
    archive: |
      {{.Stage}} {{.DeliverySummaryPath}}
YAML
PATH="$fakebin:/usr/bin:/bin" HOME="$success_home" XDG_STATE_HOME="$state_home" "$bin" run --change demo --json > run.json
run_id="$(grep -o '"run_id":"[^"]*"' run.json | head -n 1 | cut -d '"' -f 4)"
state="$(find "$state_home/wo/repos" -path "*/runs/$run_id/state.json" -print -quit)"
test -s "$state"
grep -q '"status": "done"' "$state"
grep -q '"pi:executor": "pi-session-1"' "$state"
grep -q '"pi:archiver": "pi-session-1"' "$state"
grep -q '^anthropic/claude-sonnet$' pi-models.log
grep -q '^high$' pi-thinking.log

mixed_repo="$tmp/mixed"
mixed_state="$tmp/state-mixed"
mixed_fakebin="$tmp/fakebin-mixed"
mixed_home="$tmp/home-mixed"
mkdir -p "$mixed_fakebin" "$mixed_state" "$mixed_home"
make_repo "$mixed_repo"
install_fake_oz "$mixed_fakebin"
install_fake_codex "$mixed_fakebin"
install_fake_pi "$mixed_fakebin"
ln -sf "$mixed_fakebin/pi" "$mixed_fakebin/agy"
cat > wo.yaml <<'YAML'
wo:
  workflow:
    max_review_iterations: 0
    parallel:
      enabled: false
    stages:
      planning:
        cli: pi
      execution:
        cli: codex
      archive:
        cli: codex
  prompts:
    execution: |
      {{.Stage}}
    archive: |
      {{.Stage}} {{.DeliverySummaryPath}}
YAML
unset status
PATH="$mixed_fakebin:/usr/bin:/bin" HOME="$mixed_home" XDG_STATE_HOME="$mixed_state" "$bin" run --change demo --json > mixed.out 2> mixed.err || status=$?
status="${status:-0}"
test "$status" -eq 0
if grep -q '找不到 pi 可执行文件' mixed.err; then
  cat mixed.err >&2
  exit 1
fi

missing_repo="$tmp/missing"
missing_state="$tmp/state-missing"
missing_fakebin="$tmp/fakebin-missing"
missing_home="$tmp/home-missing"
mkdir -p "$missing_fakebin" "$missing_state" "$missing_home"
make_repo "$missing_repo"
install_fake_oz "$missing_fakebin"
install_fake_codex "$missing_fakebin"
ln -sf "$missing_fakebin/codex" "$missing_fakebin/agy"
cat > wo.yaml <<'YAML'
wo:
  workflow:
    max_review_iterations: 0
    parallel:
      enabled: false
    stages:
      execution:
        cli: pi
      archive:
        cli: pi
YAML
set +e
PATH="$missing_fakebin:/usr/bin:/bin" HOME="$missing_home" XDG_STATE_HOME="$missing_state" "$bin" run --change demo --json > missing.out 2> missing.err
status=$?
set -e
test "$status" -ne 0
grep -q '找不到 pi 可执行文件' missing.err
if find "$missing_state/wo/repos" -path '*/runs/*/state.json' -print -quit 2>/dev/null | grep -q .; then
  find "$missing_state/wo/repos" -path '*/runs/*/state.json' -print >&2
  exit 1
fi
