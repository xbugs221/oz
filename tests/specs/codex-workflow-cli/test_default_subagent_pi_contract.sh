#!/usr/bin/env bash
# 文件功能目的：验证新项目默认 oz-flow.yaml 中 parallel subagent tool 使用 pi，且不暴露内部引擎字段。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/workflow/default-subagent-pi"
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

note "generate default oz-flow.yaml in a fresh project"
(
  cd "$PROJECT"
  "$OZ_BIN" flow config
) >"$RESULT_DIR/config.out" 2>"$RESULT_DIR/config.err"

cp "$PROJECT/oz-flow.yaml" "$RESULT_DIR/oz-flow.yaml"
if grep -q '^engine:' "$PROJECT/oz-flow.yaml"; then
  fail "default oz-flow.yaml should not expose an engine field"
fi
grep -q '代码库侦察员' "$PROJECT/oz-flow.yaml" || fail "default oz-flow.yaml should include implementation helper members"

member_count="$(grep -c '^[[:space:]]*- name:' "$PROJECT/oz-flow.yaml" || true)"
pi_tool_count="$(grep -c '^[[:space:]]*agent: pi$' "$PROJECT/oz-flow.yaml" || true)"
[[ "$member_count" -gt 0 ]] || fail "default oz-flow.yaml should contain subagent members"
[[ "$pi_tool_count" -eq "$member_count" ]] || fail "expected every default subagent member to write agent: pi, got $pi_tool_count/$member_count"

note "default graph contains implementation subagent nodes without duplicated planning helpers"
(
  cd "$PROJECT"
  "$OZ_BIN" flow graph --change demo --format json
) >"$RESULT_DIR/graph.json" 2>"$RESULT_DIR/graph.err"
grep -q 'implementation_context' "$RESULT_DIR/graph.json" || fail "graph should include implementation_context nodes"
grep -q '代码库侦察员' "$RESULT_DIR/graph.json" || fail "graph should include the code exploration subagent"

note "contract passed: default parallel subagent tool is pi"
