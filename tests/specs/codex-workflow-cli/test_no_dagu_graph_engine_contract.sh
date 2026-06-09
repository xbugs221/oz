#!/usr/bin/env bash
# 文件功能目的：验证 wo 公开 graph/engine 合同不再暴露 Dagu 或 legacy engine 分支。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/4-clean-dagu/graph-engine"
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
if grep -Eiq 'dagu|Dagu' "$RESULT_DIR/graph.json"; then
  fail "json graph should not expose Dagu wording"
fi

note "mermaid graph remains available"
(
  cd "$PROJECT"
  "$WO_BIN" graph --change demo --format mermaid
) >"$RESULT_DIR/graph.mmd" 2>"$RESULT_DIR/graph-mermaid.err"
grep -q 'flowchart TD' "$RESULT_DIR/graph.mmd" || fail "mermaid graph did not render a flowchart"
if grep -Eiq 'dagu|Dagu' "$RESULT_DIR/graph.mmd"; then
  fail "mermaid graph should not expose Dagu wording"
fi

note "dagu graph format is rejected and not advertised as an option"
set +e
(
  cd "$PROJECT"
  "$WO_BIN" graph --change demo --format dagu
) >"$RESULT_DIR/graph-dagu.out" 2>"$RESULT_DIR/graph-dagu.err"
dagu_graph_code=$?
set -e
[[ "$dagu_graph_code" -ne 0 ]] || fail "wo graph --format dagu should fail"
if grep -Eiq '可选.*dagu|json.*mermaid.*dagu|Dagu CLI' "$RESULT_DIR/graph-dagu.out" "$RESULT_DIR/graph-dagu.err"; then
  fail "graph error still advertises Dagu as a supported format"
fi

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

note "workflow.engine dagu is rejected before graph output"
cat >"$PROJECT/wo.yaml" <<'YAML'
wo:
  workflow:
    engine: dagu
YAML
set +e
(
  cd "$PROJECT"
  "$WO_BIN" graph --change demo --format json
) >"$RESULT_DIR/engine-dagu.out" 2>"$RESULT_DIR/engine-dagu.err"
dagu_engine_code=$?
set -e
[[ "$dagu_engine_code" -ne 0 ]] || fail "workflow.engine dagu should be rejected"
grep -Eiq 'go-dag' "$RESULT_DIR/engine-dagu.out" "$RESULT_DIR/engine-dagu.err" || fail "dagu rejection should guide the user to go-dag"
if grep -Eiq 'Dagu CLI|请安装 dagu' "$RESULT_DIR/engine-dagu.out" "$RESULT_DIR/engine-dagu.err"; then
  fail "config rejection should not route through Dagu CLI diagnostics"
fi

note "run --engine dagu is rejected without Dagu CLI lookup"
rm -f "$PROJECT/wo.yaml"
set +e
(
  cd "$PROJECT"
  "$WO_BIN" run --change demo --engine dagu --json
) >"$RESULT_DIR/run-engine-dagu.out" 2>"$RESULT_DIR/run-engine-dagu.err"
run_dagu_code=$?
set -e
[[ "$run_dagu_code" -ne 0 ]] || fail "wo run --engine dagu should fail"
grep -Eiq 'go-dag' "$RESULT_DIR/run-engine-dagu.out" "$RESULT_DIR/run-engine-dagu.err" || fail "run --engine dagu rejection should guide the user to go-dag"
if grep -Eiq 'Dagu CLI|请安装 dagu|dagu start' "$RESULT_DIR/run-engine-dagu.out" "$RESULT_DIR/run-engine-dagu.err"; then
  fail "run --engine dagu should not check or call Dagu CLI"
fi

note "contract passed: public graph and engine paths only expose go-dag"
