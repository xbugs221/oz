#!/usr/bin/env bash
# Build wo and verify public CLI commands through a temporary user repository.
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

bin="$tmp/wo"
GOBIN="$tmp/bin" go build -ldflags="-X github.com/xbugs221/oz/internal/app.Version=v9.8.7" -o "$bin" "$repo_root/cmd/wo"

work="$tmp/work"
home="$tmp/home"
state_home="$tmp/state"
mkdir -p "$work" "$home" "$state_home" "$tmp/fakebin"
cd "$work"
git init >/dev/null
git config user.email test@example.com
git config user.name Test
printf 'demo\n' > README.md
git add README.md
git commit -m init >/dev/null

PATH="/usr/bin:/bin" "$bin" --help | grep -q 'Runner JSON 命令：'
PATH="/usr/bin:/bin" "$bin" --version | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'

"$bin" config | grep -q '已创建 wo.yaml'
grep -q 'max_review_iterations' wo.yaml
grep -q 'validation:' wo.yaml
grep -q 'execution:' wo.yaml

HOME="$home" "$bin" config --global | grep -q "$home/wo.yaml"
test -s "$home/wo.yaml"
test ! -e .wo

mkdir -p docs/changes/demo
cat > "$tmp/fakebin/oz" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  list) printf '{"changes":[{"name":"demo"}]}\n' ;;
  *) printf 'unexpected oz command: %s\n' "$*" >&2; exit 2 ;;
esac
EOF
chmod +x "$tmp/fakebin/oz"

PATH="$tmp/fakebin:/usr/bin:/bin" "$bin" contract --json > contract.json
grep -q '"json":true' contract.json
grep -q '"list-changes"' contract.json

PATH="$tmp/fakebin:/usr/bin:/bin" "$bin" list-changes --json > changes.json
grep -q '"name":"demo"' changes.json

repo_key="$(basename "$PWD" | tr '[:upper:]' '[:lower:]')-$(printf '%s' "$PWD" | sha1sum | cut -c1-10)"
mkdir -p "$state_home/wo/repos/$repo_key/runs/run-1"
cat > "$state_home/wo/repos/$repo_key/runs/run-1/state.json" <<'EOF'
{"run_id":"run-1","change_name":"demo","status":"running","stage":"execution","stages":{},"paths":{},"sessions":{},"error":""}
EOF
XDG_STATE_HOME="$state_home" "$bin" status --run-id run-1 --json > status.json
grep -q '"run_id":"run-1"' status.json
grep -q '"status":"running"' status.json

XDG_STATE_HOME="$state_home" "$bin" abort --run-id run-1 --json > abort.json
grep -q '"status":"aborted"' abort.json
