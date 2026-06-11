#!/usr/bin/env bash
# 文件功能目的：验证新项目默认 wo.yaml 中 parallel subagent tool 使用 pi 而不是 legacy-agent。
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

note "generate default wo.yaml in a fresh project"
(
  cd "$PROJECT"
  "$WO_BIN" config
) >"$RESULT_DIR/config.out" 2>"$RESULT_DIR/config.err"

cp "$PROJECT/wo.yaml" "$RESULT_DIR/wo.yaml"
grep -q 'engine: go-dag' "$PROJECT/wo.yaml" || fail "default wo.yaml should keep engine: go-dag"
grep -q 'planning_context:' "$PROJECT/wo.yaml" || fail "default wo.yaml should include planning_context"
grep -q 'implementation_context:' "$PROJECT/wo.yaml" || fail "default wo.yaml should include implementation_context"

member_count="$(grep -c '^            - name:' "$PROJECT/wo.yaml" || true)"
pi_tool_count="$(grep -c '^              tool: pi$' "$PROJECT/wo.yaml" || true)"
[[ "$member_count" -gt 0 ]] || fail "default wo.yaml should contain subagent members"
[[ "$pi_tool_count" -eq "$member_count" ]] || fail "expected every default subagent member to write tool: pi, got $pi_tool_count/$member_count"
if grep -Eq '^[[:space:]]+tool: (codex|legacy-agent)$' "$PROJECT/wo.yaml"; then
  fail "default wo.yaml should not contain non-pi subagent tool"
fi

note "default graph still contains planning and implementation subagent nodes"
(
  cd "$PROJECT"
  "$WO_BIN" graph --change demo --format json
) >"$RESULT_DIR/graph.json" 2>"$RESULT_DIR/graph.err"
grep -q 'planning_context' "$RESULT_DIR/graph.json" || fail "graph should include planning_context nodes"
grep -q 'implementation_context' "$RESULT_DIR/graph.json" || fail "graph should include implementation_context nodes"
grep -q '需求分析员' "$RESULT_DIR/graph.json" || fail "graph should include the requirement-analysis subagent"
grep -q '代码库侦察员' "$RESULT_DIR/graph.json" || fail "graph should include the code exploration subagent"

note "contract passed: default parallel subagent tool is pi"
