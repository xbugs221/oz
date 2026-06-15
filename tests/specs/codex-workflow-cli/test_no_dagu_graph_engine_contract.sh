#!/usr/bin/env bash
# 文件功能目的：验证 oz flow 公开 graph 合同不暴露内部引擎或旧运行时。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/workflow/graph-engine"
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

OZ_BIN="$TMP/oz"
note "build real oz flow binary"
(cd "$ROOT" && go build -o "$OZ_BIN" ./cmd/oz) >>"$RESULT_DIR/test.log" 2>&1

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
  "$OZ_BIN" flow graph --change demo --format json
) >"$RESULT_DIR/graph.json" 2>"$RESULT_DIR/graph-json.err"
grep -q '"change_name": "demo"' "$RESULT_DIR/graph.json" || fail "json graph did not include the requested change"

note "mermaid graph remains available"
(
  cd "$PROJECT"
  "$OZ_BIN" flow graph --change demo --format mermaid
) >"$RESULT_DIR/graph.mmd" 2>"$RESULT_DIR/graph-mermaid.err"
grep -q 'flowchart TD' "$RESULT_DIR/graph.mmd" || fail "mermaid graph did not render a flowchart"
note "unknown graph format is rejected and not advertised as an option"
set +e
(
  cd "$PROJECT"
  "$OZ_BIN" flow graph --change demo --format yaml
) >"$RESULT_DIR/graph-unknown.out" 2>"$RESULT_DIR/graph-unknown.err"
unknown_graph_code=$?
set -e
[[ "$unknown_graph_code" -ne 0 ]] || fail "oz flow graph --format yaml should fail"
grep -Eiq 'json.*mermaid|mermaid.*json' "$RESULT_DIR/graph-unknown.out" "$RESULT_DIR/graph-unknown.err" || fail "graph error should advertise only current formats"

note "legacy workflow config shape is rejected before graph output"
cat >"$PROJECT/oz-flow.yaml" <<'YAML'
wo:
  workflow:
    engine: legacy
YAML
set +e
(
  cd "$PROJECT"
  "$OZ_BIN" flow graph --change demo --format json
) >"$RESULT_DIR/engine-legacy.out" 2>"$RESULT_DIR/engine-legacy.err"
legacy_code=$?
set -e
[[ "$legacy_code" -ne 0 ]] || fail "legacy workflow config shape should be rejected"
grep -Eiq '根节点 stages|root.*stages' "$RESULT_DIR/engine-legacy.out" "$RESULT_DIR/engine-legacy.err" || fail "legacy rejection should guide the user to root stages config"

note "run --engine unknown is rejected without exposing internal engine"
rm -f "$PROJECT/oz-flow.yaml"
set +e
(
  cd "$PROJECT"
  "$OZ_BIN" flow run --change demo --engine unknown --json
) >"$RESULT_DIR/run-engine-unknown.out" 2>"$RESULT_DIR/run-engine-unknown.err"
run_unknown_code=$?
set -e
[[ "$run_unknown_code" -ne 0 ]] || fail "oz flow run --engine unknown should fail"
grep -Eiq 'engine 参数已移除|参数已移除' "$RESULT_DIR/run-engine-unknown.out" "$RESULT_DIR/run-engine-unknown.err" || fail "run --engine unknown rejection should explain the removed parameter"

note "contract passed: public graph and removed engine paths hide internal engine"
