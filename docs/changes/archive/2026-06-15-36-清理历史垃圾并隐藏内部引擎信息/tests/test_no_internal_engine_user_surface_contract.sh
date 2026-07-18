#!/usr/bin/env bash
# 文件功能目的：验证内部调度实现名称不会出现在非开发用户可见的文档、配置和 CLI 输出中。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/36-cleanup"
LOG="$RESULT_DIR/internal-engine-user-surface.log"
TMP="$(mktemp -d)"

cleanup() {
  # cleanup 删除本测试创建的临时仓库和临时二进制。
  rm -rf "$TMP"
}

note() {
  # note 同时写控制台和日志，方便执行阶段复核具体失败位置。
  printf '[internal-engine-user-surface] %s\n' "$*" | tee -a "$LOG"
}

fail() {
  # fail 输出业务语义化失败原因。
  note "FAIL: $*"
  exit 1
}

assert_no_internal_engine_text() {
  # assert_no_internal_engine_text 禁止用户输出泄漏内部引擎名或旧 Dagu 名称。
  local path="$1"
  if rg -n -i '\bgo-dag\b|\bdagu\b|engine: go-dag|引擎 go-dag' "$path" >>"$LOG" 2>&1; then
    fail "$path 暴露了内部引擎或旧 Dagu 名称"
  fi
}

trap cleanup EXIT
mkdir -p "$RESULT_DIR"
: >"$LOG"

cd "$ROOT"

note "扫描用户可见文档、模板和当前规格测试"
scan_paths=(
  README.md
  docs/specs
  prompts-template
  profiles-template
  docs/changes/archive/2026-06-15-36-清理历史垃圾并隐藏内部引擎信息/tests/specs
  .github/workflows
)
if hits="$(rg -n -i --glob '!docs/changes/archive/**' '\bgo-dag\b|\bdagu\b|engine: go-dag|引擎 go-dag' "${scan_paths[@]}" 2>/dev/null || true)" && [[ -n "$hits" ]]; then
  printf '%s\n' "$hits" | tee -a "$LOG"
  fail "用户可见维护面仍描述内部引擎名称"
fi

note "构建真实 oz 二进制"
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

note "检查帮助和默认配置不暴露内部引擎"
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

note "检查 graph/status 输出不暴露内部引擎"
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

note "检查已删除的 engine 参数错误不泄漏内部实现名"
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
