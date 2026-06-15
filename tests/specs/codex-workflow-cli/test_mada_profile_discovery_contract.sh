#!/usr/bin/env bash
# Sources: 11-新增-MADA-工作流profiles
# Purpose: 验证 oz flow config 的 profile 发现入口和未知 profile 错误路径。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
result_dir="$repo_root/test-results/11-mada-profiles"
log="$result_dir/mada-profile-discovery.log"
tmpdir="$(mktemp -d)"

mkdir -p "$result_dir"
: >"$log"

cleanup() {
  rm -rf "$tmpdir"
}
trap cleanup EXIT

note() {
  printf '%s\n' "$*" | tee -a "$log"
}

fail() {
  printf 'FAIL: %s\n' "$*" | tee -a "$log" >&2
  exit 1
}

make_repo() {
  local repo="$tmpdir/repo"
  mkdir -p "$repo"
  git -C "$repo" init >/dev/null
  git -C "$repo" config user.email test@example.com
  git -C "$repo" config user.name Test
  printf 'demo\n' >"$repo/README.md"
  git -C "$repo" add README.md
  git -C "$repo" commit -m init >/dev/null
  printf '%s\n' "$repo"
}

assert_text() {
  local file="$1"
  local text="$2"
  if ! grep -Fq "$text" "$file"; then
    fail "$file 缺少期望文本: $text"
  fi
}

oz_bin="$tmpdir/wo"
note "构建真实 oz flow 二进制: $oz_bin"
go build -C "$repo_root" -o "$oz_bin" ./cmd/oz 2>&1 | tee -a "$log"
repo="$(make_repo)"
list_out="$tmpdir/list-profiles.out"
bad_out="$tmpdir/bad-profile.out"

note "运行 oz flow config --list-profiles"
(
  cd "$repo"
  "$oz_bin" config --list-profiles
) >"$list_out" 2>&1
cat "$list_out" | tee -a "$log"

for profile in default mada-code mada-decision mada-research; do
  assert_text "$list_out" "$profile"
done
for text in 代码 决策 调研; do
  assert_text "$list_out" "$text"
done
if [[ -e "$repo/oz-flow.yaml" ]]; then
  fail "oz flow config --list-profiles 不得写入 oz-flow.yaml"
fi

note "运行未知 profile 错误路径"
set +e
(
  cd "$repo"
  "$oz_bin" config --profile not-real
) >"$bad_out" 2>&1
status=$?
set -e
cat "$bad_out" | tee -a "$log"

if [[ "$status" -eq 0 ]]; then
  fail "未知 profile 必须非零退出"
fi
assert_text "$bad_out" "not-real"
assert_text "$bad_out" "mada-code"
assert_text "$bad_out" "mada-decision"
assert_text "$bad_out" "mada-research"
if [[ -e "$repo/oz-flow.yaml" ]]; then
  fail "未知 profile 错误路径不得写入 oz-flow.yaml"
fi

note "PASS"
