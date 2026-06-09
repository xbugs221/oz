#!/usr/bin/env bash
# Exercise interactive menu branches with fake oz and temporary workflow state.
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

bin="$tmp/wo"
go build -o "$bin" "$repo_root/cmd/wo"

work="$tmp/work"
state_home="$tmp/state"
mkdir -p "$work" "$state_home" "$tmp/fakebin"
cd "$work"
git init >/dev/null
git config user.email test@example.com
git config user.name Test
printf 'demo\n' > README.md
git add README.md
git commit -m init >/dev/null

cat > "$tmp/fakebin/oz" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  list) printf '{"changes":[]}\n' ;;
  *) printf 'unexpected oz command: %s\n' "$*" >&2; exit 2 ;;
esac
EOF
chmod +x "$tmp/fakebin/oz"

cat > "$tmp/fakebin/codex" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'planning called\n' > "$WO_TEST_PLANNING_MARKER"
if [ -n "${WO_PLANNING_SESSION_FILE:-}" ]; then
  printf 'planning-session\n' > "$WO_PLANNING_SESSION_FILE"
fi
EOF
chmod +x "$tmp/fakebin/codex"

repo_key="$(basename "$PWD" | tr '[:upper:]' '[:lower:]')-$(printf '%s' "$PWD" | sha1sum | cut -c1-10)"

set +e
PATH="$tmp/fakebin:/usr/bin:/bin" XDG_STATE_HOME="$state_home" WO_TEST_PLANNING_MARKER="$tmp/planning-marker.log" "$bin" > no-change.out 2> no-change.err
status=$?
set -e
test "$status" -eq 0
test -s "$tmp/planning-marker.log"
! grep -q '选择已有变更' no-change.out
test ! -e "$state_home/wo/repos/$repo_key/batches"

mkdir -p "$state_home/wo/repos/$repo_key/runs/run-1"
cat > "$state_home/wo/repos/$repo_key/runs/run-1/state.json" <<'EOF'
{"run_id":"run-1","change_name":"demo","status":"running","stage":"execution","stages":{"execution":"running"},"paths":{},"sessions":{},"error":""}
EOF

set +e
printf '2\n2\n' | PATH="$tmp/fakebin:/usr/bin:/bin" XDG_STATE_HOME="$state_home" "$bin" > abort.out 2> abort.err
status=$?
set -e
test "$status" -eq 0
grep -q '发现未完成 run：run-1' abort.out
grep -q '中止未完成 run' abort.out
grep -q '"status": "aborted"' "$state_home/wo/repos/$repo_key/runs/run-1/state.json"
