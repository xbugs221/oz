#!/usr/bin/env bash
# 文件功能目的：验证 wo 当前维护面不再保留 Dagu 运行时、规格、长期测试和隐藏 node 入口。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/5-clean-wo-legacy/no-dagu-runtime-residue"
TMP="$(mktemp -d)"

cleanup() {
  rm -rf "$TMP"
}

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

note() {
  printf '%s\n' "$*" | tee -a "$RESULT_DIR/contract.log"
}

trap cleanup EXIT
rm -rf "$RESULT_DIR"
mkdir -p "$RESULT_DIR"

WO_BIN="$TMP/wo"
note "build real wo binary"
(cd "$ROOT" && go build -o "$WO_BIN" ./cmd/wo) >>"$RESULT_DIR/contract.log" 2>&1

PROJECT="$TMP/project"
mkdir -p "$PROJECT"
(
  cd "$PROJECT"
  git init -q
  git config user.email "test@example.com"
  git config user.name "Test User"
  printf 'demo\n' > README.md
  git add README.md
  git commit -qm init
)

note "current graph formats still work"
(
  cd "$PROJECT"
  "$WO_BIN" graph --change demo --format json
) >"$RESULT_DIR/graph.json" 2>>"$RESULT_DIR/contract.log"
grep -q '"change_name": "demo"' "$RESULT_DIR/graph.json" || fail "json graph should include requested change"
(
  cd "$PROJECT"
  "$WO_BIN" graph --change demo --format mermaid
) >"$RESULT_DIR/graph.mmd" 2>>"$RESULT_DIR/contract.log"
grep -q 'flowchart TD' "$RESULT_DIR/graph.mmd" || fail "mermaid graph should render"

note "hidden wo node entry is removed"
set +e
(
  cd "$PROJECT"
  "$WO_BIN" node run-stage --run-id missing --stage execution --json
) >"$RESULT_DIR/node.out" 2>"$RESULT_DIR/node.err"
node_code=$?
set -e
[[ "$node_code" -ne 0 ]] || fail "wo node should not be a supported entry"
if ! grep -Eiq '未知命令|已移除|不支持' "$RESULT_DIR/node.out" "$RESULT_DIR/node.err"; then
  fail "wo node should fail before run-node state handling"
fi
if grep -Eiq 'run-id|state|状态|workflow_config|Dagu|dagu' "$RESULT_DIR/node.out" "$RESULT_DIR/node.err"; then
  fail "wo node still appears to enter old node execution path"
fi

note "scan current maintained files for Dagu residue"
SCAN_TARGETS=(
  "$ROOT/cmd/wo"
  "$ROOT/internal/app"
  "$ROOT/prompts-template"
  "$ROOT/README.md"
  "$ROOT/docs/specs/codex-workflow-cli/spec.md"
  "$ROOT/tests/specs/codex-workflow-cli"
)
if rg -n --hidden 'Dagu|dagu|StartDagu|ExportWorkflowDagu|ExportRunWorkflowDagu|runDagu|writeRunDagu|Dagu CLI|dagu start|--engine dagu|format dagu' "${SCAN_TARGETS[@]}" >"$RESULT_DIR/dagu-residue.txt"; then
  cat "$RESULT_DIR/dagu-residue.txt" >&2
  fail "current runtime/spec/tests still contain Dagu residue"
fi

note "scan current specs/tests for hidden node dispatch residue"
NODE_SCAN_TARGETS=(
  "$ROOT/docs/specs/codex-workflow-cli/spec.md"
  "$ROOT/tests/specs/codex-workflow-cli"
)
if rg -n --hidden 'wo node|node run-subagent|node run-stage|run-subagent --run-id|run-stage --run-id' "${NODE_SCAN_TARGETS[@]}" >"$RESULT_DIR/node-dispatch-residue.txt"; then
  cat "$RESULT_DIR/node-dispatch-residue.txt" >&2
  fail "current specs/tests still describe hidden wo node dispatch as a runtime contract"
fi

note "contract passed: current wo maintenance surface has no Dagu runtime residue"
