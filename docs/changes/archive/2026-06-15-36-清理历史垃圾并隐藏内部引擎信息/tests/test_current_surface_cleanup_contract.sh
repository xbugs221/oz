#!/usr/bin/env bash
# 文件功能目的：验证活跃源码、规格、测试和模板不再保留旧 wo、Dagu 或 legacy 后端产品合同。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/36-cleanup"
LOG="$RESULT_DIR/current-surface-cleanup.log"

note() {
  # note 输出可复核的扫描步骤和命中内容。
  printf '[current-surface-cleanup] %s\n' "$*" | tee -a "$LOG"
}

fail() {
  # fail 说明哪类旧产品合同仍在活跃维护面中。
  note "FAIL: $*"
  exit 1
}

scan_forbidden() {
  # scan_forbidden 在活跃维护面中查找禁止残留；允许调用方传入精确路径集合。
  local label="$1"
  local pattern="$2"
  shift 2
  local output
  output="$(rg -n --glob '!docs/changes/archive/**' "$pattern" "$@" 2>/dev/null || true)"
  if [[ -n "$output" ]]; then
    note "命中禁止残留：$label"
    printf '%s\n' "$output" | tee -a "$LOG"
    fail "$label"
  fi
}

mkdir -p "$RESULT_DIR"
: >"$LOG"

cd "$ROOT"

[[ ! -e cmd/wo ]] || fail "仍存在独立 cmd/wo 入口"

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

note "扫描旧 wo 命令、配置、状态目录和产品变量"
scan_forbidden "旧 wo 二进制或模块路径" 'cmd/wo|\.\/cmd/wo|go install .*cmd/wo|github\.com/xbugs221/wo' "${active_paths[@]}"
scan_forbidden "旧 wo 配置或状态目录" 'wo\.yaml|(^|/)\.wo(/|$)|XDG_STATE_HOME[^[:space:]]*/wo|state_home[^[:space:]]*/wo|/wo/repos|\bwo/repos\b' "${active_paths[@]}"
scan_forbidden "旧 wo 用户命令提示" '\bwo (status|watch|run|clean|config|restart|resume|batch|abort|update|graph|contract|list-changes)\b|`wo`|独立 `wo`|wo 命令|wo 二进制|wo CLI|wo 工作流|wo 执行器' "${active_paths[@]}"
scan_forbidden "旧 WO_* 产品变量" '\bWO_[A-Z0-9_]*|\bwo_bin\b|\bWO_BIN\b' internal tests/specs docs/specs README.md

note "扫描 Dagu 运行时和用户合同残留"
scan_forbidden "Dagu 运行时或合同残留" '\bDagu\b|\bdagu\b|StartDagu|ExportWorkflowDagu|runDagu|writeRunDagu' "${active_paths[@]}"

note "扫描 legacy 后端迁移残留"
scan_forbidden "legacy-agent 不得出现在默认配置、文档或模板" 'legacy-agent|LegacyAgent' README.md docs/specs prompts-template profiles-template .github/workflows
scan_forbidden "opencode 不得出现在当前产品合同" 'opencode|OpenCode|open""code' README.md docs/specs prompts-template profiles-template .github/workflows

note "确认当前 Go 测试仍可运行"
go test ./... -count=1 >>"$LOG" 2>&1

note "PASS"
