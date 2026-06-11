#!/usr/bin/env bash
# 验证 sealed run 使用 YAML prompt 快照、resume 不随当前配置漂移，并兼容 legacy prompt 目录。
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
{"summary":"test acceptance","required_tests":[{"id":"contract-demo","source":"change_contract","path":"docs/changes/demo/tests/demo.acceptance.test.ts","command":"true","purpose":"cover demo contract","assertions":["temporary workflow fixture exercises the script-specific business path"]}],"required_evidence":[{"id":"screenshot-demo","kind":"screenshot","path":"test-results/demo.png","purpose":"prove demo runtime"}]}
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
printf '{"type":"thread.started","thread_id":"fake-thread-%s"}\n' "$count"

case "$prompt" in
  snapshot\ execution\ execution*)
    cat > "$repo/wo.yaml" <<'YAML'
wo:
  workflow:
    max_review_iterations: 1
    parallel:
      enabled: false
  prompts:
    planning: changed planning
    execution: "changed execution {{.Stage}}\n"
    fix: "changed fix {{.Stage}}\n"
    review: "changed review {{.Stage}}\n"
    qa: "changed qa {{.Stage}}\n"
    archive: "changed archive {{.Stage}}\n"
YAML
    printf -- '- [x] task\n' > "$repo/docs/changes/demo/task.md"
    ;;
  review\ review_2*)
    review_path="$(grep -o '/tmp/[^`[:space:]]*review-2\.json' <<<"$prompt" | tail -n 1)"
    printf '{"summary":"ok","decision":"clean","checks":{"oz_aligned":true,"tasks_verified":true,"tests_meaningful":true,"implementation_scoped":true,"runtime_behavior_verified":true,"previous_findings_resolved":true},"evidence":["validation artifact passed: validation-execution-1.json","runtime evidence: Playwright screenshot test-results/demo.png"],"findings":[]}\n' > "$review_path"
    ;;
  review\ review_1*)
    review_path="$(grep -o '/tmp/[^`[:space:]]*review-1\.json' <<<"$prompt" | tail -n 1)"
    printf '{"summary":"fix","decision":"needs_fix","checks":{"oz_aligned":false,"tasks_verified":false,"tests_meaningful":false,"implementation_scoped":true,"runtime_behavior_verified":false,"previous_findings_resolved":false},"evidence":[],"findings":[{"title":"bug","severity":"major","evidence":"failed","recommendation":"fix"}]}\n' > "$review_path"
    ;;
  snapshot\ fix\ fix_1*)
    fix_path="$(grep -o '/tmp/[^`[:space:]]*fix-1-summary\.md' <<<"$prompt" | tail -n 1)"
    printf 'fixed\n' > "$fix_path"
    ;;
  qa\ qa_2*)
    qa_path="$(grep -o '/tmp/[^`[:space:]]*qa-2\.json' <<<"$prompt" | tail -n 1)"
    printf '{"summary":"qa ok","decision":"clean","evidence":["Playwright test passed with screenshot artifact test-results/demo.png"],"findings":[],"acceptance_matrix":[{"id":"contract-demo","status":"passed","artifact":"docs/changes/demo/tests/demo.acceptance.test.ts","evidence":"contract test passed"},{"id":"screenshot-demo","status":"passed","artifact":"test-results/demo.png","evidence":"screenshot artifact shows demo runtime"}]}\n' > "$qa_path"
    ;;
  archive\ archive*)
    delivery_path="$(grep -o '/tmp/[^`[:space:]]*delivery-summary\.md' <<<"$prompt" | tail -n 1)"
    printf 'done\n' > "$delivery_path"
    mkdir -p "$repo/docs/changes/archive/2026-05-05-demo"
    ;;
  legacy\ execution*)
    printf -- '- [x] task\n' > "$repo/docs/changes/demo/task.md"
    ;;
  legacy\ archive*)
    delivery_path="$(grep -o '/tmp/[^`[:space:]]*delivery-summary\.md' <<<"$prompt" | tail -n 1)"
    printf 'done\n' > "$delivery_path"
    mkdir -p "$repo/docs/changes/archive/2026-05-05-demo"
    ;;
  *)
    printf 'unexpected prompt: %s\n' "$prompt" >&2
    exit 3
    ;;
esac
EOF
chmod +x "$fakebin/codex"
ln -sf "$fakebin/codex" "$fakebin/pi"

cat > wo.yaml <<'EOF'
wo:
  workflow:
    max_review_iterations: 1
    parallel:
      enabled: false
  prompts:
    planning: planning
    execution: "snapshot execution {{.Stage}}\n"
    fix: "snapshot fix {{.Stage}} {{.FixSummaryPath}}\n"
    review: "review {{.Stage}} {{.ReviewPath}}\n"
    qa: "qa {{.Stage}} {{.QAPath}}\n"
    archive: "archive {{.Stage}} {{.DeliverySummaryPath}}\n"
EOF

PATH="$fakebin:/usr/bin:/bin" HOME="$home" XDG_STATE_HOME="$state_home" "$bin" run --change demo --json > run.json
run_id="$(grep -o '"run_id":"[^"]*"' run.json | head -n 1 | cut -d '"' -f 4)"
test -n "$run_id"
run_dir="$(find "$state_home/wo/repos" -path "*/runs/$run_id" -type d -print -quit)"
test -s "$run_dir/prompt-snapshot.yaml"
test ! -e "$run_dir/prompts"
grep -q 'execution:' "$run_dir/prompt-snapshot.yaml"
grep -q 'fix:' "$run_dir/prompt-snapshot.yaml"
grep -q 'snapshot execution' prompt-1.txt
grep -q 'snapshot fix fix_1' prompt-3.txt
! grep -q 'changed fix' prompt-3.txt

legacy_run="legacy-run"
legacy_dir="$(dirname "$run_dir")/$legacy_run"
mkdir -p "$legacy_dir"
cat > "$legacy_dir/prompt-snapshot.yaml" <<'EOF'
prompts:
  planning: "legacy {{.Stage}}\n"
  execution: "legacy {{.Stage}}\n"
  review: "legacy {{.Stage}} {{.ReviewPath}}\n"
  qa: "legacy {{.Stage}} {{.QAPath}}\n"
  fix: "legacy {{.Stage}} {{.FixSummaryPath}}\n"
  archive: "legacy {{.Stage}} {{.DeliverySummaryPath}}\n"
EOF
printf -- '- [ ] task\n' > docs/changes/demo/task.md
rm -f prompt-*.txt .fake-codex-count run.json
git add docs/changes/demo/task.md wo.yaml
git commit -m legacy-reset >/dev/null
head="$(git rev-parse HEAD)"
cat > "$legacy_dir/state.json" <<EOF
{"run_id":"$legacy_run","change_name":"demo","sealed":true,"status":"running","stage":"execution","baseline_head":"$head","baseline_diff":"","sessions":{},"stages":{},"paths":{},"workflow_config":{"max_review_iterations":0,"stages":{"execution":{"tool":"codex","reasoning":"low","fast":false},"archive":{"tool":"codex","reasoning":"low","fast":false}},"validation":{}}}
EOF
PATH="$fakebin:/usr/bin:/bin" HOME="$home" XDG_STATE_HOME="$state_home" "$bin" resume --run-id "$legacy_run" --json > "$tmp/legacy.json"
grep -q '"status": "done"' "$legacy_dir/state.json"
grep -q 'legacy execution' prompt-1.txt
