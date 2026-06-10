#!/usr/bin/env bash
# 文件功能目的：验证 wo config 的 profile 发现入口和未知 profile 错误路径。
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
  # note 记录 profile 发现路径的真实 CLI 输出。
  printf '%s\n' "$*" | tee -a "$log"
}

fail() {
  # fail 输出业务级失败原因，便于定位 profile 发现合同缺口。
  printf 'FAIL: %s\n' "$*" | tee -a "$log" >&2
  exit 1
}

make_repo() {
  # make_repo 创建真实 git 仓库，覆盖 GitRoot 相关行为。
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
  # assert_text 验证命令输出包含用户可见 profile 名称或中文用途说明。
  local file="$1"
  local text="$2"
  if ! grep -Fq "$text" "$file"; then
    fail "$file 缺少期望文本: $text"
  fi
}

wo_bin="$tmpdir/wo"
note "构建真实 wo 二进制: $wo_bin"
go build -C "$repo_root" -o "$wo_bin" ./cmd/wo 2>&1 | tee -a "$log"
repo="$(make_repo)"
list_out="$tmpdir/list-profiles.out"
bad_out="$tmpdir/bad-profile.out"

note "运行 wo config --list-profiles"
(
  cd "$repo"
  "$wo_bin" config --list-profiles
) >"$list_out" 2>&1
cat "$list_out" | tee -a "$log"

for profile in default mada-code mada-decision mada-research; do
  assert_text "$list_out" "$profile"
done
for text in 代码 决策 调研; do
  assert_text "$list_out" "$text"
done
if [[ -e "$repo/wo.yaml" ]]; then
  fail "wo config --list-profiles 不得写入 wo.yaml"
fi

note "运行未知 profile 错误路径"
set +e
(
  cd "$repo"
  "$wo_bin" config --profile not-real
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
if [[ -e "$repo/wo.yaml" ]]; then
  fail "未知 profile 错误路径不得写入 wo.yaml"
fi

note "PASS"
