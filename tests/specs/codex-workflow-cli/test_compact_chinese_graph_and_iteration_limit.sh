#!/usr/bin/env bash
# 文件功能目的：验证默认最大审核迭代数为 5，且 oz flow graph 输出紧凑中文 Mermaid 图。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/workflow/compact-chinese-graph"
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

OZ_BIN="$TMP/wo"
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

note "generate default oz-flow.yaml and verify iteration budget"
(
  cd "$PROJECT"
  "$OZ_BIN" flow config
) >"$RESULT_DIR/config.out" 2>"$RESULT_DIR/config.err"
cp "$PROJECT/oz-flow.yaml" "$RESULT_DIR/oz-flow.yaml"
grep -q 'max_review_iterations: 5' "$PROJECT/oz-flow.yaml" || fail "default max_review_iterations should be 5"
if grep -q 'max_review_iterations: 30' "$PROJECT/oz-flow.yaml"; then
  fail "default max_review_iterations should no longer be 30"
fi
if grep -q '^engine:' "$PROJECT/oz-flow.yaml"; then
  fail "default oz-flow.yaml should not expose an engine field"
fi

note "render mermaid graph and verify it is compact"
(
  cd "$PROJECT"
  "$OZ_BIN" flow graph --change demo --format mermaid
) >"$RESULT_DIR/graph.mmd" 2>"$RESULT_DIR/graph.err"
grep -q 'flowchart TD' "$RESULT_DIR/graph.mmd" || fail "mermaid graph should render a flowchart"

if grep -Eq 'review_2|qa_2|fix_2|review_5|qa_5|fix_5' "$RESULT_DIR/graph.mmd"; then
  fail "mermaid graph should not repeat review/qa/fix nodes per iteration"
fi

if grep -Eq 'subagent:|fan-in|planning_context|implementation_context|before_review|before_qa|before_execution' "$RESULT_DIR/graph.mmd"; then
  fail "mermaid visible labels should not mix internal English subagent/group names"
fi

grep -q '代码库侦察员' "$RESULT_DIR/graph.mmd" || fail "graph should keep the Chinese code-exploration subagent label"
grep -q '外部资料研究员' "$RESULT_DIR/graph.mmd" || fail "graph should keep the Chinese external-research subagent label"
grep -Eq '5|五' "$RESULT_DIR/graph.mmd" || fail "graph should communicate the 5-iteration review budget"

note "contract passed: default iteration budget is 5 and graph is compact Chinese"
