#!/usr/bin/env bash
# 验证人类 status 固定展示规划行，已有提案直接执行时显示未知。
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

WO="$TMP/wo"
(cd "$ROOT" && go build -o "$WO" ./cmd/wo)
export HOME="$TMP/home"
export XDG_STATE_HOME="$TMP/state"
mkdir -p "$HOME"

REPO="$TMP/repo"
mkdir -p "$REPO/docs/changes/demo"
git -C "$REPO" init -q
git -C "$REPO" config user.email test@example.com
git -C "$REPO" config user.name Test
cat >"$REPO/docs/changes/demo/task.md" <<'TASK'
- [ ] demo
TASK
touch "$REPO/README.md"
git -C "$REPO" add .
git -C "$REPO" commit -q -m init

cat >"$REPO/wo.yaml" <<'YAML'
wo:
  workflow:
    max_review_iterations: 0
  prompts:
    execution: |
      execution
      state={{.StatePath}}
    archive: |
      archive
      delivery={{.DeliverySummaryPath}}
YAML

mkdir -p "$TMP/bin"
cat >"$TMP/bin/oz" <<'OZ'
#!/usr/bin/env bash
if [ "$1" = "validate" ]; then exit 0; fi
if [ "$1" = "status" ]; then echo '{"status":"incomplete","tasks":{"total":1,"done":0}}'; exit 0; fi
if [ "$1" = "list" ]; then echo "demo"; exit 0; fi
OZ
cat >"$TMP/bin/codex" <<'CODEX'
#!/usr/bin/env bash
prompt=$(cat)
state=$(printf '%s\n' "$prompt" | awk -F= '/^state=/{print $2; exit}')
delivery=$(printf '%s\n' "$prompt" | awk -F= '/^delivery=/{print $2; exit}')
if [ -n "$state" ]; then
  dir=$(dirname "$state")
  cat >"$dir/execution-summary.md" <<'EOF'
done
EOF
fi
if [ -n "$delivery" ]; then
  mkdir -p "$(dirname "$delivery")"
  cat >"$delivery" <<'EOF'
done
EOF
fi
echo '{"type":"thread.started","thread_id":"thread-demo"}'
CODEX
chmod +x "$TMP/bin/oz" "$TMP/bin/codex"
export PATH="$TMP/bin:$PATH"

HASH=$(printf '%s' "$REPO" | sha1sum | cut -c1-10)
RUN_DIR="$XDG_STATE_HOME/wo/repos/repo-$HASH/runs/demo-run"
mkdir -p "$RUN_DIR"
cat >"$RUN_DIR/state.json" <<'JSON'
{
  "run_id": "demo-run",
  "change_name": "demo",
  "sealed": true,
  "status": "running",
  "stage": "execution",
  "error": "",
  "baseline_head": "HEAD",
  "baseline_diff": "",
  "sessions": {},
  "stages": {"planning": "completed"},
  "paths": {},
  "workflow_config": {
    "max_review_iterations": 0,
    "stages": {
      "planning": {"tool": "codex", "reasoning": "xhigh"},
      "execution": {"tool": "codex", "reasoning": "low"},
      "archive": {"tool": "codex", "reasoning": "low"}
    },
    "validation": {"max_attempts_per_stage": 3}
  }
}
JSON

STATUS=$(cd "$REPO" && "$WO" status)
printf '%s\n' "$STATUS" | grep -q -- "规划阶段 - ✓ -"
printf '%s\n' "$STATUS" | grep -q -- "执行阶段"
