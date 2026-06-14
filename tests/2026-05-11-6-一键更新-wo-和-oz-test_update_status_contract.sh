#!/usr/bin/env bash
# Verifies wo status update hints stay human-only and best-effort.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

BIN="$TMP/wo"
go build -o "$BIN" "$ROOT/cmd/wo"

REPO="$TMP/repo"
mkdir -p "$REPO/docs/changes/demo"
printf '# demo\n' >"$REPO/README.md"
git -C "$REPO" init -q
git -C "$REPO" config user.email test@example.com
git -C "$REPO" config user.name test
git -C "$REPO" add .
git -C "$REPO" commit -q -m init

STATE_HOME="$TMP/state"
RUN_DIR="$STATE_HOME/wo/repos/repo-$(printf '%s' "$REPO" | sha1sum | cut -c1-10)/runs/run-1"
mkdir -p "$RUN_DIR"
cat >"$RUN_DIR/state.json" <<'JSON'
{
  "run_id": "run-1",
  "change_name": "demo",
  "sealed": true,
  "status": "running",
  "stage": "execution",
  "error": "",
  "sessions": {},
  "stages": {},
  "paths": {},
  "workflow_config": {
    "max_review_iterations": 3,
    "stages": {"execution": {"tool": "codex", "reasoning": "low"}},
    "validation": {}
  }
}
JSON

CACHE="$TMP/cache/update-check.json"
mkdir -p "$(dirname "$CACHE")"
cat >"$CACHE" <<'JSON'
{
  "checked_at": "2999-01-01T00:00:00Z",
  "ttl_seconds": 21600,
  "tools": {
    "wo": {"current": "v1.0.0", "latest": "v1.1.0", "update_available": true}
  }
}
JSON

cd "$REPO"
status_out="$(XDG_STATE_HOME="$STATE_HOME" WO_UPDATE_CACHE="$CACHE" "$BIN" status)"
printf '%s\n' "$status_out" | grep -- "执行 - → -" >/dev/null
printf '%s\n' "$status_out" | grep "更新可用" >/dev/null
printf '%s\n' "$status_out" | grep "wo update" >/dev/null

json_out="$(XDG_STATE_HOME="$STATE_HOME" WO_UPDATE_CACHE="$CACHE" "$BIN" status --run-id run-1 --json)"
printf '%s\n' "$json_out" | grep '"run_id":"run-1"' >/dev/null
if printf '%s\n' "$json_out" | grep "更新可用\\|wo update" >/dev/null; then
  echo "JSON status leaked update hint" >&2
  exit 1
fi

cat >"$TMP/offline-cache.json" <<JSON
{
  "checked_at": "2999-01-01T00:00:00Z",
  "ttl_seconds": 21600,
  "tools": {}
}
JSON
offline_out="$(XDG_STATE_HOME="$STATE_HOME" WO_UPDATE_CACHE="$TMP/offline-cache.json" "$BIN" status)"
printf '%s\n' "$offline_out" | grep -- "执行 - → -" >/dev/null
if printf '%s\n' "$offline_out" | grep "更新失败\\|GitHub\\|更新可用" >/dev/null; then
  echo "offline status should stay silent about update checks" >&2
  exit 1
fi

FAKE_PATH="$TMP/fake-path"
mkdir -p "$FAKE_PATH"
cat >"$FAKE_PATH/oz" <<'SH'
#!/bin/sh
exit 0
SH
chmod +x "$FAKE_PATH/oz"
cat >"$TMP/empty-oz-cache.json" <<JSON
{
  "checked_at": "2999-01-01T00:00:00Z",
  "ttl_seconds": 21600,
  "tools": {}
}
JSON
empty_oz_out="$(PATH="$FAKE_PATH:$PATH" XDG_STATE_HOME="$STATE_HOME" WO_UPDATE_CACHE="$TMP/empty-oz-cache.json" "$BIN" status)"
printf '%s\n' "$empty_oz_out" | grep -- "执行 - → -" >/dev/null
if printf '%s\n' "$empty_oz_out" | grep "panic\\|更新失败\\|更新可用" >/dev/null; then
  echo "empty oz --version should not break status" >&2
  exit 1
fi
