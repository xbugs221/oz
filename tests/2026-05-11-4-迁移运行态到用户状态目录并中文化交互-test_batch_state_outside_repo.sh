#!/usr/bin/env bash
# Build wo and verify interactive batch submission stores queue state outside the repository.
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d)"
cleanup() {
  # Give the detached batch worker a short chance to finish writing state.
  if [ -n "${batch_state:-}" ] && [ -f "$batch_state" ]; then
    for _ in $(seq 1 50); do
      status="$(python3 - "$batch_state" <<'PY' 2>/dev/null || true
import json
import sys

with open(sys.argv[1], encoding="utf-8") as fh:
    print(json.load(fh).get("status", ""))
PY
)"
      [ "$status" != "running" ] && break
      sleep 0.1
    done
  fi
  for _ in $(seq 1 10); do
    rm -rf "$tmp" 2>/dev/null && return
    sleep 0.1
  done
  rm -rf "$tmp"
}
trap cleanup EXIT

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
mkdir -p docs/changes/1-a docs/changes/2-b

cat > "$fakebin/oz" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  list) printf '{"changes":[{"name":"2-b"},{"name":"1-a"}]}\n' ;;
  validate) printf '{"valid":true,"change":"%s","errors":[],"warnings":[],"artifacts":{}}\n' "$2" ;;
  status) printf '{"change":"%s","status":"complete","tasks":{"total":1,"done":1}}\n' "$2" ;;
esac
EOF
chmod +x "$fakebin/oz"

cat > "$fakebin/codex" <<'EOF'
#!/usr/bin/env bash
printf '{"type":"thread.started","thread_id":"fake-thread"}\n'
EOF
chmod +x "$fakebin/codex"
ln -sf "$fakebin/codex" "$fakebin/pi"

printf '2\n1-2\n' | PATH="$fakebin:/usr/bin:/bin" HOME="$home" XDG_STATE_HOME="$state_home" "$bin" >"$tmp/wo-batch.out"

test ! -e .wo/batches
batch_state="$(find "$state_home/wo/repos" -path '*/batches/*/state.json' -print -quit)"
test -s "$batch_state"
grep -q '"1-a"' "$batch_state"
grep -q '"2-b"' "$batch_state"
grep -q '"status": "running"' "$batch_state"
