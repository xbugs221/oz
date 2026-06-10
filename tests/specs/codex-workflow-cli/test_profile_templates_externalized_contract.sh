#!/usr/bin/env bash
# Sources: 11-新增-MADA-工作流profiles
# Purpose: 验证 wo 默认工作流和 MADA profiles 的 subagent/prompt 配置已从 Go 硬编码迁移到内置 YAML 模板。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
result_dir="$repo_root/test-results/11-mada-profiles"
log="$result_dir/profile-templates-externalized.log"
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

assert_file() {
  local path="$1"
  [[ -f "$path" ]] || fail "缺少模板文件: $path"
}

assert_contains() {
  local file="$1"
  local text="$2"
  if ! grep -Fq "$text" "$file"; then
    fail "$file 缺少期望内容: $text"
  fi
}

assert_production_go_not_contains() {
  local text="$1"
  local file
  for file in "$repo_root"/internal/app/*.go; do
    case "$file" in
      *_test.go) continue ;;
    esac
    if grep -Fq "$text" "$file"; then
      fail "生产 Go 源码仍硬编码默认模板文本: $text in $file"
    fi
  done
}

new_repo() {
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

template_dir="$repo_root/profiles-template"
default_template="$template_dir/default.yaml"
code_template="$template_dir/mada-code.yaml"
decision_template="$template_dir/mada-decision.yaml"
research_template="$template_dir/mada-research.yaml"

note "校验 profile YAML 模板文件存在"
for template in "$default_template" "$code_template" "$decision_template" "$research_template"; do
  assert_file "$template"
done

note "校验 default.yaml 保存默认 subagent 和 prompt 配置语义"
for text in "wo:" "parallel:" "prompts:" "planning_context:" "implementation_context:" "review:" "qa:" "需求分析员" "代码库侦察员" "外部资料研究员"; do
  assert_contains "$default_template" "$text"
done

note "校验 MADA profile 模板使用同一目录维护"
for template in "$code_template" "$decision_template" "$research_template"; do
  assert_contains "$template" "wo:"
  assert_contains "$template" "parallel:"
  assert_contains "$template" "planning_context:"
  assert_contains "$template" "review:"
done
for role in 需求澄清员 约束建模员 候选方案研究员 反方评审员 运维部署评审员 学习路线评审员 证据审计员; do
  assert_contains "$decision_template" "$role"
done

note "校验生产 Go 源码不再硬编码默认 subagent 角色文本"
for text in 需求分析员 代码库侦察员 外部资料研究员 "找出需求歧义、风险和遗漏" "搜索现有模块、测试入口和实现约定"; do
  assert_production_go_not_contains "$text"
done

wo_bin="$tmpdir/wo"
note "构建真实 wo 二进制: $wo_bin"
go build -C "$repo_root" -o "$wo_bin" ./cmd/wo 2>&1 | tee -a "$log"

repo="$(new_repo)"
note "运行默认 wo config，确认 default.yaml 语义仍生成标准 wo.yaml"
(
  cd "$repo"
  "$wo_bin" config
) 2>&1 | tee -a "$log"

yaml="$repo/wo.yaml"
[[ -f "$yaml" ]] || fail "默认 wo config 未生成 wo.yaml"
for text in "parallel:" "enabled: true" "planning_context:" "implementation_context:" "需求分析员"; do
  assert_contains "$yaml" "$text"
done

note "运行 wo graph 验证默认模板生成的配置可加载"
(
  cd "$repo"
  "$wo_bin" graph --change "11-template-externalized-demo" --format json
) >"$repo/graph.json" 2>>"$log"
assert_contains "$repo/graph.json" '"type": "subagent"'
assert_contains "$repo/graph.json" '"type": "fanin"'

note "PASS"
