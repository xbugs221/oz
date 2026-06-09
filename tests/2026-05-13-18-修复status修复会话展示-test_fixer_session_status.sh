#!/usr/bin/env bash
# 验证修复阶段按 backend 保存 fixer session，并在 status 中展示真实修复会话。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

bin="$tmp/wo"
go build -o "$bin" "$repo_root/cmd/wo"

make_repo() {
  local work="$1"
  mkdir -p "$work/docs/changes/demo"
  git -C "$work" init >/dev/null
  git -C "$work" config user.email test@example.com
  git -C "$work" config user.name Test
  printf 'demo\n' > "$work/README.md"
  cat > "$work/docs/changes/demo/task.md" <<'TASK'
- [ ] implement demo
TASK
  cat > "$work/docs/changes/demo/acceptance.json" <<'JSON'
{"summary":"test acceptance","required_tests":[{"id":"contract-demo","source":"change_contract","path":"docs/changes/demo/tests/demo.acceptance.test.ts","command":"true","purpose":"cover demo contract"}],"required_evidence":[{"id":"screenshot-demo","kind":"screenshot","path":"test-results/demo.png","purpose":"prove demo runtime"}]}
JSON
  git -C "$work" add .
  git -C "$work" commit -m init >/dev/null
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
      printf '{"change":"%s","status":"ready","tasks":{"total":1,"done":1}}\n' "$2"
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

install_fake_agent() {
  local dir="$1"
  local tool="$2"
  cat > "$dir/$tool" <<'AGENT'
#!/usr/bin/env bash
set -euo pipefail
tool="$(basename "$0")"
repo="$PWD"
resume=""
prompt=""
if [[ "$tool" == "codex" ]]; then
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --cd) repo="$2"; shift 2 ;;
      resume) resume="$2"; shift 2 ;;
      -) shift ;;
      *) shift ;;
    esac
  done
  prompt="$(cat)"
else
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --dir) repo="$2"; shift 2 ;;
      --session) resume="$2"; shift 2 ;;
      --mode|--format|--model|-m|--variant|--thinking) shift 2 ;;
      --dangerously-skip-permissions) shift ;;
      run) shift ;;
      *) prompt="$1"; shift ;;
    esac
  done
fi
count_file="$repo/.fake-$tool-count"
count=0
if [[ -f "$count_file" ]]; then
  count="$(cat "$count_file")"
fi
count=$((count + 1))
printf '%s' "$count" > "$count_file"
printf '%s\n' "$prompt" > "$repo/$tool-prompt-$count.txt"
if [[ -n "$resume" ]]; then
  printf '%s\n' "$resume" >> "$repo/$tool-resume.log"
fi

session="$tool-session-$count"
if grep -q '^fix_1$' <<<"$prompt"; then
  session="$tool-fixer"
elif grep -q '^fix_2$' <<<"$prompt"; then
  test "$resume" = "$tool-fixer"
  session="$tool-fixer"
elif [[ -n "$resume" ]]; then
  session="$resume"
fi

case "$tool" in
  codex) printf '{"type":"thread.started","thread_id":"%s"}\n' "$session" ;;
  opencode) printf '{"type":"session.updated","sessionID":"%s"}\n' "$session" ;;
  pi) printf '{"type":"session","id":"%s"}\n' "$session" ;;
esac

if grep -q '^acceptance$' <<<"$prompt"; then
  path="$(awk '{for (i=1; i<=NF; i++) if ($i ~ /acceptance\.json$/) print $i}' <<<"$prompt" | tail -n 1)"
  printf '%s\n' '{"summary":"test acceptance","required_tests":[{"id":"contract-demo","source":"change_contract","path":"docs/changes/demo/tests/demo.acceptance.test.ts","command":"true","purpose":"cover demo contract"}],"required_evidence":[{"id":"screenshot-demo","kind":"screenshot","path":"test-results/demo.png","purpose":"prove demo runtime"}]} ' > "$path"
elif grep -q '^execution$' <<<"$prompt"; then
  printf -- '- [x] implement demo\n' > "$repo/docs/changes/demo/task.md"
elif grep -q '^review_1$' <<<"$prompt"; then
  path="$(awk '{for (i=1; i<=NF; i++) if ($i ~ /review-1\.json$/) print $i}' <<<"$prompt" | tail -n 1)"
  printf '{"summary":"fix 1","decision":"needs_fix","checks":{"oz_aligned":false,"tasks_verified":true,"tests_meaningful":false,"implementation_scoped":true,"runtime_behavior_verified":false,"previous_findings_resolved":false},"evidence":[],"findings":[{"title":"bug 1","severity":"major","evidence":"failed","recommendation":"fix"}]}\n' > "$path"
elif grep -q '^fix_1$' <<<"$prompt"; then
  path="$(awk '{for (i=1; i<=NF; i++) if ($i ~ /fix-1-summary\.md$/) print $i}' <<<"$prompt" | tail -n 1)"
  printf 'fixed 1\n' > "$path"
elif grep -q '^review_2$' <<<"$prompt"; then
  path="$(awk '{for (i=1; i<=NF; i++) if ($i ~ /review-2\.json$/) print $i}' <<<"$prompt" | tail -n 1)"
  printf '{"summary":"fix 2","decision":"needs_fix","checks":{"oz_aligned":false,"tasks_verified":true,"tests_meaningful":false,"implementation_scoped":true,"runtime_behavior_verified":false,"previous_findings_resolved":false},"evidence":[],"findings":[{"title":"bug 2","severity":"major","evidence":"failed","recommendation":"fix"}]}\n' > "$path"
elif grep -q '^fix_2$' <<<"$prompt"; then
  path="$(awk '{for (i=1; i<=NF; i++) if ($i ~ /fix-2-summary\.md$/) print $i}' <<<"$prompt" | tail -n 1)"
  printf 'fixed 2\n' > "$path"
elif grep -q '^review_3$' <<<"$prompt"; then
  path="$(awk '{for (i=1; i<=NF; i++) if ($i ~ /review-3\.json$/) print $i}' <<<"$prompt" | tail -n 1)"
  printf '{"summary":"ok","decision":"clean","checks":{"oz_aligned":true,"tasks_verified":true,"tests_meaningful":true,"implementation_scoped":true,"runtime_behavior_verified":true,"previous_findings_resolved":true},"evidence":["validation artifact passed: validation-execution-1.json","runtime evidence: Playwright screenshot test-results/demo.png"],"findings":[]}\n' > "$path"
elif grep -q '^qa_3$' <<<"$prompt"; then
  path="$(awk '{for (i=1; i<=NF; i++) if ($i ~ /qa-3\.json$/) print $i}' <<<"$prompt" | tail -n 1)"
  printf '{"summary":"qa ok","decision":"clean","evidence":["Playwright test passed with screenshot artifact test-results/demo.png"],"findings":[],"acceptance_matrix":[{"id":"contract-demo","status":"passed","artifact":"docs/changes/demo/tests/demo.acceptance.test.ts","evidence":"contract test passed"},{"id":"screenshot-demo","status":"passed","artifact":"test-results/demo.png","evidence":"screenshot artifact shows demo runtime"}]}\n' > "$path"
elif grep -q '^archive$' <<<"$prompt"; then
  path="$(awk '{for (i=1; i<=NF; i++) if ($i ~ /delivery-summary\.md$/) print $i}' <<<"$prompt" | tail -n 1)"
  printf 'done\n' > "$path"
  mkdir -p "$repo/docs/changes/archive/2026-05-13-demo"
fi
AGENT
  chmod +x "$dir/$tool"
}

run_backend_case() {
  local tool="$1"
  local work="$tmp/work-$tool"
  local state_home="$tmp/state-$tool"
  local home="$tmp/home-$tool"
  local fakebin="$tmp/fakebin-$tool"
  mkdir -p "$work" "$state_home" "$home" "$fakebin"
  make_repo "$work"
  install_fake_oz "$fakebin"
  install_fake_agent "$fakebin" "$tool"
  cat > "$work/wo.yaml" <<YAML
wo:
  workflow:
    max_review_iterations: 3
    parallel:
      enabled: false
    stages:
      execution:
        cli: $tool
      review:
        cli: $tool
      qa:
        cli: $tool
      fix:
        cli: $tool
      archive:
        cli: $tool
  prompts:
    execution: |
      {{.Stage}}
      {{.ReviewPath}}
      {{.FixSummaryPath}}
      {{.DeliverySummaryPath}}
    review: |
      {{.Stage}}
      {{.ReviewPath}}
      {{.FixSummaryPath}}
      {{.DeliverySummaryPath}}
    qa: |
      {{.Stage}}
      {{.QAPath}}
    fix: |
      {{.Stage}}
      {{.ReviewPath}}
      {{.FixSummaryPath}}
      {{.DeliverySummaryPath}}
    archive: |
      {{.Stage}}
      {{.ReviewPath}}
      {{.FixSummaryPath}}
      {{.DeliverySummaryPath}}
YAML
  cd "$work"
  PATH="$fakebin:/usr/bin:/bin" HOME="$home" XDG_STATE_HOME="$state_home" "$bin" run --change demo --json > run.json
  run_id="$(grep -o '"run_id":"[^"]*"' run.json | head -n 1 | cut -d '"' -f 4)"
  state="$(find "$state_home/wo/repos" -path "*/runs/$run_id/state.json" -print -quit)"
  test -s "$state"
  grep -q "\"$tool:fixer\": \"$tool-fixer\"" "$state"
  ! grep -q "\"$tool:executor\": \"$tool-fixer\"" "$state"
  grep -q "^$tool-fixer$" "$work/$tool-resume.log"
  PATH="$fakebin:/usr/bin:/bin" HOME="$home" XDG_STATE_HOME="$state_home" "$bin" status -w1 > status.txt
  grep -q -- "修正阶段 $tool-fixer ✓✓" status.txt
  ! grep -q -- "修正阶段 未知" status.txt
}

run_legacy_case() {
  local work="$tmp/work-legacy"
  local state_home="$tmp/state-legacy"
  local home="$tmp/home-legacy"
  mkdir -p "$work" "$state_home" "$home"
  make_repo "$work"
  cd "$work"
  repo_name="$(basename "$PWD" | tr '[:upper:]' '[:lower:]' | tr -cs 'a-z0-9' '-' | sed 's/^-//;s/-$//')"
  repo_key="$repo_name-$(printf '%s' "$PWD" | sha1sum | cut -c1-10)"
  run_dir="$state_home/wo/repos/$repo_key/runs/legacy-run"
  mkdir -p "$run_dir"
  cat > "$run_dir/state.json" <<'JSON'
{
  "run_id": "legacy-run",
  "change_name": "demo",
  "status": "running",
  "stage": "review_2",
  "stages": {
    "execution": "completed",
    "review_1": "completed",
    "fix_1": "completed"
  },
  "paths": {},
  "sessions": {
    "codex:executor": "executor-thread",
    "codex:reviewer": "reviewer-thread"
  },
  "error": "",
  "workflow_config": {
    "max_review_iterations": 2,
    "stages": {
      "execution": {"tool":"codex","reasoning":"low","fast":false},
      "review_1": {"tool":"codex","reasoning":"high","fast":false},
      "fix_1": {"tool":"codex","reasoning":"low","fast":false},
      "review_2": {"tool":"codex","reasoning":"high","fast":false},
      "fix_2": {"tool":"codex","reasoning":"low","fast":false},
      "archive": {"tool":"codex","reasoning":"low","fast":false}
    },
    "validation": {"max_attempts_per_stage": 3, "commands": []}
  }
}
JSON
  HOME="$home" XDG_STATE_HOME="$state_home" "$bin" status -w1 > legacy-status.txt
  grep -q -- "修正阶段 - ✓" legacy-status.txt
  ! grep -q -- "修正阶段 executor-thread" legacy-status.txt
}

run_backend_case codex
run_backend_case opencode
run_backend_case pi
run_legacy_case
