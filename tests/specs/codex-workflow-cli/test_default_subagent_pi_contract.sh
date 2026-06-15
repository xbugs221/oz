#!/usr/bin/env bash
# 文件功能目的：验证新项目默认 oz-flow.yaml 中 parallel subagent tool 使用 pi 而不是 legacy-agent。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/go-dag/default-subagent-pi"
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
note "build real oz flow binary"
(cd "$ROOT" && go build -o "$WO_BIN" ./cmd/oz) >>"$RESULT_DIR/test.log" 2>&1

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

note "generate default oz-flow.yaml in a fresh project"
(
  cd "$PROJECT"
  "$WO_BIN" config
) >"$RESULT_DIR/config.out" 2>"$RESULT_DIR/config.err"

cp "$PROJECT/oz-flow.yaml" "$RESULT_DIR/oz-flow.yaml"
grep -q 'engine: go-dag' "$PROJECT/oz-flow.yaml" || fail "default oz-flow.yaml should keep engine: go-dag"
grep -q 'implementation_context:' "$PROJECT/oz-flow.yaml" || fail "default oz-flow.yaml should include implementation_context"

member_count="$(grep -c '^            - name:' "$PROJECT/oz-flow.yaml" || true)"
pi_tool_count="$(grep -c '^              tool: pi$' "$PROJECT/oz-flow.yaml" || true)"
[[ "$member_count" -gt 0 ]] || fail "default oz-flow.yaml should contain subagent members"
[[ "$pi_tool_count" -eq "$member_count" ]] || fail "expected every default subagent member to write tool: pi, got $pi_tool_count/$member_count"
if grep -Eq '^[[:space:]]+tool: (codex|legacy-agent)$' "$PROJECT/oz-flow.yaml"; then
  fail "default oz-flow.yaml should not contain non-pi subagent tool"
fi

note "default graph contains implementation subagent nodes without duplicated planning helpers"
(
  cd "$PROJECT"
  "$WO_BIN" flow graph --change demo --format json
) >"$RESULT_DIR/graph.json" 2>"$RESULT_DIR/graph.err"
grep -q 'implementation_context' "$RESULT_DIR/graph.json" || fail "graph should include implementation_context nodes"
grep -q '代码库侦察员' "$RESULT_DIR/graph.json" || fail "graph should include the code exploration subagent"

note "contract passed: default parallel subagent tool is pi"
