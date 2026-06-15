#!/usr/bin/env bash
# 文件功能目的：验证用户可见维护面不暴露内部引擎名或旧产品合同。
# Sources: 36-清理历史垃圾并隐藏内部引擎信息
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/codex-workflow-cli/user-visible-surface-cleanup"
LOG="$RESULT_DIR/contract.log"
TMP="$(mktemp -d)"

cleanup() {
  # cleanup 删除本测试创建的临时仓库和临时二进制。
  rm -rf "$TMP"
}

note() {
  # note 写入测试步骤和命中内容，作为用户可见面清理合同证据。
  printf '[user-visible-surface-cleanup] %s\n' "$*" | tee -a "$LOG"
}

fail() {
  # fail 用业务语义报告仍暴露内部实现或旧产品合同的具体原因。
  note "FAIL: $*"
  exit 1
}

scan_forbidden() {
  # scan_forbidden 在指定活跃维护路径中查找禁止出现的旧合同文本。
  local label="$1"
  local pattern="$2"
  shift 2
  local output
  output="$(rg -n -i --glob '!docs/changes/archive/**' --glob '!tests/specs/codex-workflow-cli/test_user_visible_surface_cleanup_contract.sh' "$pattern" "$@" 2>/dev/null || true)"
  if [[ -n "$output" ]]; then
    note "命中禁止残留：$label"
    printf '%s\n' "$output" | tee -a "$LOG"
    fail "$label"
  fi
}

assert_no_internal_engine_text() {
  # assert_no_internal_engine_text 确认真实 CLI 输出没有泄漏内部 engine 名称。
  local path="$1"
  if rg -n -i '\bgo-dag\b|\bdagu\b|engine: go-dag|引擎 go-dag' "$path" >>"$LOG" 2>&1; then
    fail "$path 暴露了内部引擎或旧 Dagu 名称"
  fi
}

trap cleanup EXIT
mkdir -p "$RESULT_DIR"
: >"$LOG"

cd "$ROOT"

active_paths=(
  README.md
  docs/specs
  prompts-template
  profiles-template
  tests/specs
  .github/workflows
)

note "扫描用户文档、规格、模板、发布门禁和规格测试"
scan_forbidden "内部引擎名称不得进入用户可见面" '\bgo-dag\b|\bdagu\b|engine: go-dag|引擎 go-dag' "${active_paths[@]}"
scan_forbidden "旧 wo 配置、命令和状态目录不得进入活跃维护面" 'cmd/wo|\.\/cmd/wo|wo\.yaml|(^|/)\.wo(/|$)|/wo/repos|\bwo (status|watch|run|clean|config|restart|resume|batch|abort|update|graph|contract|list-changes)\b|`wo`|wo 命令|wo 二进制|wo CLI|wo 工作流|wo 执行器' "${active_paths[@]}"
scan_forbidden "旧 WO_* 产品变量不得进入活跃维护面" '\bWO_[A-Z0-9_]*|\bwo_bin\b|\bWO_BIN\b' internal tests/specs docs/specs README.md
scan_forbidden "legacy 后端不得进入默认配置、文档或模板" 'legacy-agent|LegacyAgent|opencode|OpenCode|open""code' README.md docs/specs prompts-template profiles-template .github/workflows

note "构建真实 oz 二进制并检查帮助、配置、graph、status、错误输出"
OZ_BIN="$TMP/oz"
go build -o "$OZ_BIN" ./cmd/oz >>"$LOG" 2>&1

PROJECT="$TMP/project"
mkdir -p "$PROJECT/docs/changes/demo"
(
  cd "$PROJECT"
  git init -q
  git config user.email "test@example.com"
  git config user.name "Test User"
  printf 'demo\n' > README.md
  git add README.md
  git commit -qm init
)

"$OZ_BIN" flow --help >"$RESULT_DIR/flow-help.txt" 2>&1
assert_no_internal_engine_text "$RESULT_DIR/flow-help.txt"
(
  cd "$PROJECT"
  "$OZ_BIN" flow config >"$RESULT_DIR/flow-config.out" 2>&1
)
assert_no_internal_engine_text "$RESULT_DIR/flow-config.out"
assert_no_internal_engine_text "$PROJECT/oz-flow.yaml"
if rg -n '^engine:' "$PROJECT/oz-flow.yaml" >>"$LOG" 2>&1; then
  fail "默认 oz-flow.yaml 不应包含用户可见 engine 字段"
fi

(
  cd "$PROJECT"
  "$OZ_BIN" flow graph --change demo --format json >"$RESULT_DIR/graph.json" 2>"$RESULT_DIR/graph-json.err"
  "$OZ_BIN" flow graph --change demo --format mermaid >"$RESULT_DIR/graph.mmd" 2>"$RESULT_DIR/graph-mermaid.err"
  "$OZ_BIN" flow status >"$RESULT_DIR/status.txt" 2>&1 || true
)
assert_no_internal_engine_text "$RESULT_DIR/graph.json"
assert_no_internal_engine_text "$RESULT_DIR/graph-json.err"
assert_no_internal_engine_text "$RESULT_DIR/graph.mmd"
assert_no_internal_engine_text "$RESULT_DIR/graph-mermaid.err"
assert_no_internal_engine_text "$RESULT_DIR/status.txt"

set +e
(
  cd "$PROJECT"
  "$OZ_BIN" flow run --change demo --engine unknown --json
) >"$RESULT_DIR/run-engine-unknown.out" 2>"$RESULT_DIR/run-engine-unknown.err"
run_code=$?
set -e
[[ "$run_code" -ne 0 ]] || fail "未知 engine 参数不应启动成功"
assert_no_internal_engine_text "$RESULT_DIR/run-engine-unknown.out"
assert_no_internal_engine_text "$RESULT_DIR/run-engine-unknown.err"

note "PASS"
