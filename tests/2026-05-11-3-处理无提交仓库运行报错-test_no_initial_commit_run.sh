#!/usr/bin/env bash
# Build wo and verify sealed runs fail clearly before state creation in a git repo without commits.
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

bin="$tmp/wo"
go build -o "$bin" "$repo_root/cmd/wo"

fakebin="$tmp/fakebin"
mkdir -p "$fakebin"
cat > "$fakebin/oz" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  validate) printf '{"valid":true,"errors":[],"warnings":[],"artifacts":{}}\n' ;;
  status) printf '{"change":"demo","status":"incomplete","tasks":{"total":1,"done":0}}\n' ;;
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
printf 'agent started\n' > "$repo/agent-started"
printf '{"type":"thread.started","thread_id":"fake-thread"}\n'
EOF
chmod +x "$fakebin/codex"
ln -sf "$fakebin/codex" "$fakebin/pi"

new_repo() {
  local work="$1"
  mkdir -p "$work/docs/changes/demo" "$work/.wo/cmd"
  cd "$work"
  git init >/dev/null
  git config user.email test@example.com
  git config user.name Test
  cat > docs/changes/demo/task.md <<'EOF'
- [ ] task
EOF
  cat > .wo/cmd/wo-start.md <<'EOF'
{{.Stage}}
EOF
  cat > .wo/cmd/wo-done.md <<'EOF'
{{.Stage}}
EOF
}

human_repo="$tmp/human"
new_repo "$human_repo"
set +e
PATH="$fakebin:/usr/bin:/bin" "$bin" --run demo >human.out 2>human.err
status=$?
set -e
test "$status" -ne 0
grep -q '首次 git commit' human.err
test ! -d .wo/runs

broken_index_repo="$tmp/broken-index"
new_repo "$broken_index_repo"
printf 'demo\n' > README.md
git add .
git commit -m init >/dev/null
printf 'bad' > .git/index
set +e
PATH="$fakebin:/usr/bin:/bin" "$bin" --run demo >broken-index.out 2>broken-index.err
status=$?
set -e
test "$status" -ne 0
grep -q 'git status --porcelain 失败' broken-index.err
grep -q 'index file smaller than expected' broken-index.err
if grep -q '首次 git commit' broken-index.err; then
  cat broken-index.err >&2
  exit 1
fi
test ! -d .wo/runs
test ! -e agent-started

tree_repo="$tmp/tree-head"
new_repo "$tree_repo"
printf 'demo\n' > README.md
git add .
git commit -m init >/dev/null
tree="$(git rev-parse 'HEAD^{tree}')"
branch="$(git symbolic-ref -q HEAD)"
printf '%s' "$tree" > ".git/$branch"
set +e
PATH="$fakebin:/usr/bin:/bin" "$bin" --run demo >tree.out 2>tree.err
status=$?
set -e
test "$status" -ne 0
grep -q 'git rev-parse --verify HEAD 失败' tree.err
grep -q 'expected commit type' tree.err
if grep -q '首次 git commit' tree.err; then
  cat tree.err >&2
  exit 1
fi
test ! -d .wo/runs
test ! -e agent-started

json_repo="$tmp/json"
new_repo "$json_repo"
set +e
PATH="$fakebin:/usr/bin:/bin" "$bin" run --change demo --json >json.out 2>json.err
status=$?
set -e
test "$status" -ne 0
grep -q '首次 git commit' json.err
test ! -s json.out
test ! -d .wo/runs

broken_repo="$tmp/broken"
new_repo "$broken_repo"
printf 'demo\n' > README.md
git add .
git commit -m init >/dev/null
branch="$(git symbolic-ref -q HEAD)"
printf 'notasha' > ".git/$branch"
set +e
PATH="$fakebin:/usr/bin:/bin" "$bin" --run demo >broken.out 2>broken.err
status=$?
set -e
test "$status" -ne 0
grep -q 'git rev-parse --verify HEAD 失败' broken.err
grep -q 'Needed a single revision' broken.err
if grep -q '首次 git commit' broken.err; then
  cat broken.err >&2
  exit 1
fi
test ! -d .wo/runs
