#!/usr/bin/env bash
# Verifies wo status shows session-role progress while JSON status stays stable.
set -euo pipefail

repo="$(mktemp -d)"
state_home="$(mktemp -d)"
home="$(mktemp -d)"
tmp="$(mktemp -d)"
trap 'rm -rf "$repo" "$state_home" "$home" "$tmp"' EXIT

git -C "$repo" init -q
git -C "$repo" config user.email test@example.com
git -C "$repo" config user.name Tester
mkdir -p "$repo/docs/changes/demo"
printf '# demo\n' > "$repo/README.md"
git -C "$repo" add .
git -C "$repo" commit -q -m init

bin="${WO_BIN:-$tmp/wo}"
if [[ ! -x "$bin" ]]; then
  go build -o "$bin" ./cmd/wo
fi
cd "$repo"
repo_name="$(basename "$PWD" | tr '[:upper:]' '[:lower:]' | tr -cs 'a-z0-9' '-' | sed 's/^-//;s/-$//')"
repo_key="$repo_name-$(printf '%s' "$PWD" | sha1sum | cut -c1-10)"
run_dir="$state_home/wo/repos/$repo_key/runs/demo-run"
mkdir -p "$run_dir"
cat > "$run_dir/state.json" <<'JSON'
{
  "run_id": "demo-run",
  "change_name": "demo",
  "status": "running",
  "stage": "archive",
  "stages": {
    "execution": "completed",
    "review_1": "completed",
    "fix_1": "completed",
    "review_2": "completed"
  },
  "paths": {},
  "sessions": {
    "codex:executor": "executor-thread",
    "codex:reviewer": "reviewer-thread",
    "codex:archiver": "archiver-thread"
  },
  "error": "",
  "workflow_config": {
    "max_review_iterations": 2,
    "stages": {
      "execution": {"tool":"codex","reasoning":"low","fast":false},
      "review_1": {"tool":"codex","reasoning":"high","fast":false},
      "fix_1": {"tool":"codex","reasoning":"low","fast":false},
      "review_2": {"tool":"codex","reasoning":"high","fast":false},
      "archive": {"tool":"codex","reasoning":"low","fast":false}
    },
    "validation": {"max_attempts_per_stage": 3, "commands": []}
  }
}
JSON

HOME="$home" XDG_STATE_HOME="$state_home" "$bin" status > status.txt
grep -q -- '执行 executor-thread ✓' status.txt
grep -q -- '审核 .*reviewer-thread .*✓2' status.txt
grep -q -- '归档 archiver-thread →' status.txt
! grep -q -- '归档 executor-thread' status.txt

HOME="$home" XDG_STATE_HOME="$state_home" "$bin" status --run-id demo-run --json > status.json
grep -q '"run_id":"demo-run"' status.json
grep -q '"stage":"archive"' status.json
grep -q '"sessions":' status.json
! grep -q 'runId' status.json
