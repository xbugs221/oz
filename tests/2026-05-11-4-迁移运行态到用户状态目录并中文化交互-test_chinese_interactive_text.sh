#!/usr/bin/env bash
# Build wo and verify primary human-facing help and menu text is Chinese.
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
git add README.md
git commit -m init >/dev/null
mkdir -p docs/changes/demo

cat > "$fakebin/oz" <<'EOF'
#!/usr/bin/env bash
printf '{"changes":[{"name":"demo"}]}\n'
EOF
chmod +x "$fakebin/oz"

PATH="/usr/bin:/bin" "$bin" --help > help.txt
grep -q '用法：' help.txt
grep -q '人类交互命令：' help.txt
grep -q 'Runner JSON 命令：' help.txt

printf '2\n1\n' | PATH="$fakebin:/usr/bin:/bin" HOME="$home" XDG_STATE_HOME="$state_home" "$bin" > menu.txt 2>menu.err || true
grep -q '进入规划阶段' menu.txt
grep -q '选择已有变更' menu.txt
! grep -q 'is not a valid oz change' menu.err

repo_key="$(basename "$PWD" | tr '[:upper:]' '[:lower:]')-$(printf '%s' "$PWD" | sha1sum | cut -c1-10)"
mkdir -p "$state_home/wo/repos/$repo_key/runs/run-1"
cat > "$state_home/wo/repos/$repo_key/runs/run-1/state.json" <<'EOF'
{"run_id":"run-1","change_name":"demo","status":"running","stage":"execution","stages":{},"paths":{},"sessions":{},"error":""}
EOF
printf '2\n2\n1\n' | PATH="$fakebin:/usr/bin:/bin" HOME="$home" XDG_STATE_HOME="$state_home" "$bin" > unfinished.txt 2>unfinished.err || true
grep -q '发现未完成 run：run-1' unfinished.txt
grep -q '恢复未完成 run' unfinished.txt
grep -q '中止未完成 run' unfinished.txt
! grep -q 'is not a valid oz change' unfinished.err
