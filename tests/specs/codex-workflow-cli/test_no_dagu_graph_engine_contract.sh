#!/usr/bin/env bash
# 文件功能目的：验证 wo 公开 graph/engine 合同只暴露当前 go-dag 工作流。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/go-dag/graph-engine"
TMP="$(mktemp -d)"

cleanup() {
  rm -rf "$TMP"
}

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

note() {
  printf '%s\n' "$*" | tee -a "$RESULT_DIR/test.log"
}

trap cleanup EXIT
rm -rf "$RESULT_DIR"
mkdir -p "$RESULT_DIR"

WO_BIN="$TMP/wo"
note "build real wo binary"
(cd "$ROOT" && go build -o "$WO_BIN" ./cmd/wo) >>"$RESULT_DIR/test.log" 2>&1

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

note "json graph remains available"
(
  cd "$PROJECT"
  "$WO_BIN" graph --change demo --format json
) >"$RESULT_DIR/graph.json" 2>"$RESULT_DIR/graph-json.err"
grep -q '"change_name": "demo"' "$RESULT_DIR/graph.json" || fail "json graph did not include the requested change"

note "mermaid graph remains available"
(
  cd "$PROJECT"
  "$WO_BIN" graph --change demo --format mermaid
) >"$RESULT_DIR/graph.mmd" 2>"$RESULT_DIR/graph-mermaid.err"
grep -q 'flowchart TD' "$RESULT_DIR/graph.mmd" || fail "mermaid graph did not render a flowchart"
note "unknown graph format is rejected and not advertised as an option"
set +e
(
  cd "$PROJECT"
  "$WO_BIN" graph --change demo --format yaml
) >"$RESULT_DIR/graph-unknown.out" 2>"$RESULT_DIR/graph-unknown.err"
unknown_graph_code=$?
set -e
[[ "$unknown_graph_code" -ne 0 ]] || fail "wo graph --format yaml should fail"
grep -Eiq 'json.*mermaid|mermaid.*json' "$RESULT_DIR/graph-unknown.out" "$RESULT_DIR/graph-unknown.err" || fail "graph error should advertise only current formats"

note "workflow.engine legacy is rejected before graph output"
cat >"$PROJECT/wo.yaml" <<'YAML'
wo:
  workflow:
    engine: legacy
YAML
set +e
(
  cd "$PROJECT"
  "$WO_BIN" graph --change demo --format json
) >"$RESULT_DIR/engine-legacy.out" 2>"$RESULT_DIR/engine-legacy.err"
legacy_code=$?
set -e
[[ "$legacy_code" -ne 0 ]] || fail "workflow.engine legacy should be rejected"
grep -Eiq 'go-dag' "$RESULT_DIR/engine-legacy.out" "$RESULT_DIR/engine-legacy.err" || fail "legacy rejection should guide the user to go-dag"

note "run --engine unknown is rejected with go-dag guidance"
rm -f "$PROJECT/wo.yaml"
set +e
(
  cd "$PROJECT"
  "$WO_BIN" run --change demo --engine unknown --json
) >"$RESULT_DIR/run-engine-unknown.out" 2>"$RESULT_DIR/run-engine-unknown.err"
run_unknown_code=$?
set -e
[[ "$run_unknown_code" -ne 0 ]] || fail "wo run --engine unknown should fail"
grep -Eiq 'go-dag' "$RESULT_DIR/run-engine-unknown.out" "$RESULT_DIR/run-engine-unknown.err" || fail "run --engine unknown rejection should guide the user to go-dag"

note "contract passed: public graph and engine paths only expose go-dag"
