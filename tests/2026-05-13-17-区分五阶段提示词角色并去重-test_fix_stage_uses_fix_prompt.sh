#!/usr/bin/env bash
# 验证 review 需要修复后，fix 阶段 agent 输入使用独立 fix prompt。
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
cat > docs/changes/demo/task.md <<'TASKS'
- [ ] task
TASKS
cat > docs/changes/demo/acceptance.json <<'JSON'
{"summary":"test acceptance","required_tests":[{"id":"contract-demo","source":"change_contract","path":"docs/changes/demo/tests/demo.acceptance.test.ts","command":"true","purpose":"cover demo contract","assertions":["temporary workflow fixture exercises the script-specific business path"]}],"required_evidence":[{"id":"screenshot-demo","kind":"screenshot","path":"test-results/demo.png","purpose":"prove demo runtime"}]}
JSON
git add .
git commit -m init >/dev/null

cat > "$fakebin/oz" <<'OZ'
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
OZ
chmod +x "$fakebin/oz"

cat > "$fakebin/codex" <<'CODEX'
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
printf '{"type":"thread.started","thread_id":"fake-thread-%s"}\n' "$count"

case "$prompt" in
  *"oz-exec"*)
    printf -- '- [x] task\n' > "$repo/docs/changes/demo/task.md"
    ;;
  *"delivery-summary.md"*)
    delivery_path="$(grep -o '/tmp/[^`[:space:]]*delivery-summary\.md' <<<"$prompt" | tail -n 1)"
    printf 'done\n' > "$delivery_path"
    mkdir -p "$repo/docs/changes/archive/2026-05-05-demo"
    ;;
  *"qa-2.json"*)
    qa_path="$(grep -o '/tmp/[^`[:space:]]*qa-2\.json' <<<"$prompt" | tail -n 1)"
    printf '{"summary":"qa ok","decision":"clean","evidence":["Playwright test passed with screenshot artifact test-results/demo.png"],"findings":[],"acceptance_matrix":[{"id":"contract-demo","status":"passed","artifact":"docs/changes/demo/tests/demo.acceptance.test.ts","evidence":"contract test passed"},{"id":"screenshot-demo","status":"passed","artifact":"test-results/demo.png","evidence":"screenshot artifact shows demo runtime"}]}\n' > "$qa_path"
    ;;
  *"fix-1-summary.md"*)
    fix_path="$(grep -o '/tmp/[^`[:space:]]*fix-1-summary\.md' <<<"$prompt" | tail -n 1)"
    printf 'fixed\n' > "$fix_path"
    ;;
  *"review-2.json"*)
    review_path="$(grep -o '/tmp/[^`[:space:]]*review-2\.json' <<<"$prompt" | tail -n 1)"
    printf '{"summary":"ok","decision":"clean","checks":{"oz_aligned":true,"tasks_verified":true,"tests_meaningful":true,"implementation_scoped":true,"runtime_behavior_verified":true,"previous_findings_resolved":true},"evidence":["validation artifact passed: validation-execution-1.json","runtime evidence: Playwright screenshot test-results/demo.png"],"findings":[]}\n' > "$review_path"
    ;;
  *"review-1.json"*)
    review_path="$(grep -o '/tmp/[^`[:space:]]*review-1\.json' <<<"$prompt" | tail -n 1)"
    printf '{"summary":"fix","decision":"needs_fix","checks":{"oz_aligned":false,"tasks_verified":false,"tests_meaningful":false,"implementation_scoped":true,"runtime_behavior_verified":false,"previous_findings_resolved":false},"evidence":[],"findings":[{"title":"bug","severity":"major","evidence":"failed","recommendation":"fix"}]}\n' > "$review_path"
    ;;
  *"acceptance.json"*)
    acceptance_path="$(grep -o '/tmp/[^`[:space:]]*acceptance\.json' <<<"$prompt" | tail -n 1)"
    printf '%s\n' '{"summary":"test acceptance","required_tests":[{"id":"contract-demo","source":"change_contract","path":"docs/changes/demo/tests/demo.acceptance.test.ts","command":"true","purpose":"cover demo contract","assertions":["temporary workflow fixture exercises the script-specific business path"]}],"required_evidence":[{"id":"screenshot-demo","kind":"screenshot","path":"test-results/demo.png","purpose":"prove demo runtime"}]} ' > "$acceptance_path"
    ;;
  *)
    printf 'unexpected prompt: %s\n' "$prompt" >&2
    exit 3
    ;;
esac
CODEX
chmod +x "$fakebin/codex"
ln -sf "$fakebin/codex" "$fakebin/pi"
ln -sf "$fakebin/codex" "$fakebin/agy"

cat > wo.yaml <<'YAML'
wo:
  workflow:
    parallel:
      enabled: false
YAML

PATH="$fakebin:/usr/bin:/bin" HOME="$home" XDG_STATE_HOME="$state_home" "$bin" run --change demo --json > run.json
run_id="$(grep -o '"run_id":"[^"]*"' run.json | head -n 1 | cut -d '"' -f 4)"
test -n "$run_id"
run_dir="$(find "$state_home/wo/repos" -path "*/runs/$run_id" -type d -print -quit)"

grep -q 'review-1.json' prompt-3.txt
grep -q 'fix-1-summary.md' prompt-3.txt
! grep -q '# wo start' prompt-3.txt
grep -q '"codex:fixer": "fake-thread-3"' "$run_dir/state.json"
