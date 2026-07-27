#!/usr/bin/env bash
# 文件功能目的：验证默认质量循环不设轮次上限，且 oz flow graph 输出紧凑中文 Mermaid 图。
# Sources: 45-收敛全量自查与QA定向修复闭环
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

note "generate default oz-flow.yaml without a fixed iteration budget"
(
  cd "$PROJECT"
  "$OZ_BIN" flow config
) >"$RESULT_DIR/config.out" 2>"$RESULT_DIR/config.err"
cp "$PROJECT/oz-flow.yaml" "$RESULT_DIR/oz-flow.yaml"
if grep -q 'max_repair_iterations:' "$PROJECT/oz-flow.yaml"; then
  fail "new default config should not emit deprecated max_repair_iterations"
fi
if grep -q 'max_review_iterations:' "$PROJECT/oz-flow.yaml"; then
  fail "new config should not emit legacy max_review_iterations"
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

if grep -Eq 'repair_[1-9]|qa_[1-9]|review_[1-9]|fix_[1-9]|第[0-9]+轮|[0-9]+轮上限' "$RESULT_DIR/graph.mmd"; then
  fail "mermaid graph should use an unbounded template without legacy finite stages"
fi

if grep -Eq 'subagent:|fan-in|planning_context|implementation_context|before_review|before_qa|before_execution' "$RESULT_DIR/graph.mmd"; then
  fail "mermaid visible labels should not mix internal English subagent/group names"
fi

grep -q '全量自查 audit_N' "$RESULT_DIR/graph.mmd" || fail "graph should show the pre-QA full audit loop"
grep -q '定向修复 targeted_repair_N' "$RESULT_DIR/graph.mmd" || fail "graph should show the QA-targeted repair loop"
grep -Eq '独立(QA|测试)' "$RESULT_DIR/graph.mmd" || fail "graph should keep QA as an independent gate"
grep -q '环境阻塞' "$RESULT_DIR/graph.mmd" || fail "graph should show recoverable environment blocking"
grep -q '停滞阻塞' "$RESULT_DIR/graph.mmd" || fail "graph should show recoverable stalled blocking"

note "contract passed: default quality loop is unbounded and graph is compact Chinese"
