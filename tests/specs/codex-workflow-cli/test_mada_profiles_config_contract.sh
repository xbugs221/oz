#!/usr/bin/env bash
# Sources: 11-新增-MADA-工作流profiles
# Purpose: 验证 oz flow config 的 MADA profiles 能生成标准 oz-flow.yaml，并可被 oz flow graph 真实读取。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
result_dir="$repo_root/test-results/11-mada-profiles"
log="$result_dir/mada-profiles-config.log"
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

new_repo() {
  local name="$1"
  local repo="$tmpdir/$name"
  mkdir -p "$repo"
  git -C "$repo" init >/dev/null
  git -C "$repo" config user.email test@example.com
  git -C "$repo" config user.name Test
  printf 'demo\n' >"$repo/README.md"
  git -C "$repo" add README.md
  git -C "$repo" commit -m init >/dev/null
  printf '%s\n' "$repo"
}

assert_contains() {
  local file="$1"
  local text="$2"
  if ! grep -Fq "$text" "$file"; then
    fail "$file 缺少期望内容: $text"
  fi
}

assert_profile_config() {
  local oz_bin="$1"
  local profile="$2"
  local repo
  repo="$(new_repo "$profile")"
  local template="$repo_root/profiles-template/$profile.yaml"

  [[ -f "$template" ]] || fail "$profile 缺少内置 YAML 模板: $template"
  assert_contains "$template" "stages:"
  if rg -n 'parallel:|subagents:|before:' "$template" >>"$log"; then
    fail "$profile 模板不应重新引入已移除的固定子代理配置"
  fi

  note "运行 oz flow config --profile $profile"
  (
    cd "$repo"
    "$oz_bin" flow config --profile "$profile"
  ) 2>&1 | tee -a "$log"

  local yaml="$repo/oz-flow.yaml"
  [[ -f "$yaml" ]] || fail "$profile 未生成 oz-flow.yaml"
  assert_contains "$yaml" "stages:"
  assert_contains "$yaml" "execution:"
  assert_contains "$yaml" "repair:"
  assert_contains "$yaml" "qa:"
  assert_contains "$yaml" "archive:"
  if rg -n 'parallel:|subagents:|before:|agent: pi' "$yaml" >>"$log"; then
    fail "$profile oz-flow.yaml 不应包含固定外置子代理"
  fi

  note "运行 oz flow graph 验证 $profile 可加载"
  (
    cd "$repo"
    "$oz_bin" flow graph --change "11-${profile}-演示" --format json
  ) >"$repo/graph.json" 2>>"$log"

  assert_contains "$repo/graph.json" '"type": "main_stage"'
  if rg -n '"type": "(subagent|fanin)"|parallel-' "$repo/graph.json" >>"$log"; then
    fail "$profile graph 不应包含固定外置子代理节点或产物"
  fi
}

oz_bin="$tmpdir/wo"
note "构建真实 oz flow 二进制: $oz_bin"
go build -C "$repo_root" -o "$oz_bin" ./cmd/oz 2>&1 | tee -a "$log"

for profile in mada-code mada-decision mada-research; do
  assert_profile_config "$oz_bin" "$profile"
done

note "PASS"
