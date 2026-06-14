#!/usr/bin/env bash
# 文件功能目的：验证活跃产品面已经删除 wo 旧命名、旧配置、旧状态目录和兼容发布合同。
# Sources: 30-彻底合并wo为oz-flow
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"

note() {
  # note 记录扫描范围和命中结果，作为旧产品面清理证据。
  printf '%s\n' "$*"
}

fail() {
  # fail 输出旧命名残留原因并终止测试。
  note "FAIL: $*"
  exit 1
}

scan_forbidden() {
  # scan_forbidden 在指定路径中查找禁止出现的旧 wo 产品合同。
  local pattern="$1"
  shift
  local output
  output="$(rg -n --glob '!docs/changes/archive/**' --glob '!tests/specs/codex-workflow-cli/test_no_wo_legacy_surface_contract.sh' --glob '!tests/specs/codex-workflow-cli/test_single_oz_flow_binary_contract.sh' "$pattern" "$@" 2>/dev/null || true)"
  if [[ -n "$output" ]]; then
    printf '%s\n' "$output"
    fail "发现禁止残留：$pattern"
  fi
}

cd "$repo_root"

[[ ! -e cmd/wo ]] || fail "仍存在 cmd/wo"

note "检查模板文件名不再使用 wo-*"
if fd '^wo-' prompts-template profiles-template 2>/dev/null | grep -q .; then
  fd '^wo-' prompts-template profiles-template
  fail "模板文件名仍使用 wo-*"
fi

active_paths=(
  cmd
  internal
  prompts-template
  profiles-template
  README.md
  docs/specs
  tests/specs
  .github/workflows
  go.mod
  go.sum
)

note "扫描活跃产品路径中的旧命令、旧配置和旧状态合同"
scan_forbidden 'cmd/wo|\.\/cmd\/wo|go install .*\/cmd\/wo|github\.com/xbugs221/wo' "${active_paths[@]}"
scan_forbidden 'github\.com/xbugs221/o\./cmd/oz' "${active_paths[@]}"
scan_forbidden 'wo\.yaml|XDG_STATE_HOME.*wo|state_home.*wo|/wo/repos|\\bwo/repos\\b' "${active_paths[@]}"
scan_forbidden '\\bwo (status|watch|run|clean|config|restart|resume|batch|abort|update|graph|contract|list-changes)\\b' "${active_paths[@]}"
scan_forbidden '`wo`|独立 `wo`|wo 命令|wo 二进制|wo CLI|wo 工作流|wo 执行器' "${active_paths[@]}"
scan_forbidden '^wo:|wo\.workflow|wo\.prompts|wo\.exe|同一 checkout 构建 oz 和 wo|当前 wo' README.md docs/specs .github/workflows

note "检查 oz flow 新命名已经出现在用户文档和规格中"
rg -n 'oz flow' README.md docs/specs tests/specs .github/workflows >/dev/null || fail "活跃文档或测试未体现 oz flow"

note "PASS"
