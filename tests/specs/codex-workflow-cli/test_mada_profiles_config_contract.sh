#!/usr/bin/env bash
# Sources: 11-新增-MADA-工作流profiles
# Purpose: 验证 wo config 的 MADA profiles 能生成标准 wo.yaml，并可被 wo graph 真实读取。
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
  local wo_bin="$1"
  local profile="$2"
  local repo
  repo="$(new_repo "$profile")"
  local template="$repo_root/profiles-template/$profile.yaml"

  [[ -f "$template" ]] || fail "$profile 缺少内置 YAML 模板: $template"
  assert_contains "$template" "stages:"
  assert_contains "$template" "parallel:"

  note "运行 wo config --profile $profile"
  (
    cd "$repo"
    "$wo_bin" config --profile "$profile"
  ) 2>&1 | tee -a "$log"

  local yaml="$repo/wo.yaml"
  [[ -f "$yaml" ]] || fail "$profile 未生成 wo.yaml"
  assert_contains "$yaml" "parallel: true"
  assert_contains "$yaml" "stages:"
  assert_contains "$yaml" "execution:"
  assert_contains "$yaml" "review:"
  assert_contains "$yaml" "qa:"
  assert_contains "$yaml" "before:"

  local member_count
  local pi_agent_count
  member_count="$(grep -c '^      - name:' "$yaml" || true)"
  pi_agent_count="$(grep -c '^        agent: pi$' "$yaml" || true)"
  [[ "$member_count" -gt 0 ]] || fail "$profile wo.yaml 缺少 subagent members"
  [[ "$pi_agent_count" -eq "$member_count" ]] || fail "$profile wo.yaml 必须为每个 subagent member 显式写 agent: pi，当前 $pi_agent_count/$member_count"
  if grep -Eq '^[[:space:]]+agent: legacy-agent$' "$yaml"; then
    fail "$profile wo.yaml 不应包含 legacy-agent subagent"
  fi

  note "运行 wo graph 验证 $profile 可加载"
  (
    cd "$repo"
    "$wo_bin" graph --change "11-${profile}-演示" --format json
  ) >"$repo/graph.json" 2>>"$log"

  assert_contains "$repo/graph.json" '"type": "subagent"'
  assert_contains "$repo/graph.json" '"type": "fanin"'
  assert_contains "$repo/graph.json" "implementation_context"
}

wo_bin="$tmpdir/wo"
note "构建真实 wo 二进制: $wo_bin"
go build -C "$repo_root" -o "$wo_bin" ./cmd/wo 2>&1 | tee -a "$log"

for profile in mada-code mada-decision mada-research; do
  assert_profile_config "$wo_bin" "$profile"
done

decision_repo="$tmpdir/mada-decision"
decision_yaml="$decision_repo/wo.yaml"
decision_graph="$decision_repo/graph.json"

note "校验 mada-decision 包含当前内置 MADA 角色"
for role in 代码库侦察员 外部资料研究员 目标核对审核员 测试有效性审核员 CLI/API\ 测试员 回归场景测试员; do
  assert_contains "$decision_yaml" "name: $role"
  assert_contains "$decision_graph" "$role"
done

note "PASS"
