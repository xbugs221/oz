#!/usr/bin/env bash
# 文件功能目的：验证默认质量循环限制全量自查轮次，且 oz flow graph 输出紧凑中文 Mermaid 图。
# Sources: 45-收敛全量自查与QA定向修复闭环, 46-验证升级后动态质量循环
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

# prompt_block 从生成配置中提取指定提示词块，避免跨角色关键词造成假阳性。
prompt_block() {
  local prompt_name="$1"
  awk -v prompt_name="$prompt_name" '
    $0 == "  " prompt_name ": |" { inside = 1; next }
    inside && /^  [a-z_]+: \|$/ { exit }
    inside { print }
  ' "$PROJECT/oz-flow.yaml"
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

note "generate default oz-flow.yaml with an audit iteration budget"
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
grep -q '^max_audit_iterations: 3$' "$PROJECT/oz-flow.yaml" ||
  fail "new default config should cap pre-QA audits at 3"
if grep -q '^engine:' "$PROJECT/oz-flow.yaml"; then
  fail "default oz-flow.yaml should not expose an engine field"
fi
REPAIR_PROMPT="$(prompt_block repair)"
QA_PROMPT="$(prompt_block qa)"
rg -q '`pre_qa_audit`.*`audit_N`' <<<"$REPAIR_PROMPT"
rg -q '`qa_targeted_repair`.*`targeted_repair_N`' <<<"$REPAIR_PROMPT"
rg -q 'repairer 不能自行归档，clean 后仍须独立 QA 放行' <<<"$REPAIR_PROMPT"
rg -q '使用独立 QA 会话' <<<"$QA_PROMPT"

note "render mermaid graph and verify it is compact"
(
  cd "$PROJECT"
  "$OZ_BIN" flow graph --change demo --format mermaid
) >"$RESULT_DIR/graph.mmd" 2>"$RESULT_DIR/graph.err"
grep -q 'flowchart TD' "$RESULT_DIR/graph.mmd" || fail "mermaid graph should render a flowchart"

if grep -Eq 'repair_[1-9]|qa_[1-9]|review_[1-9]|fix_[1-9]' "$RESULT_DIR/graph.mmd"; then
  fail "mermaid graph should use a dynamic template without expanded legacy finite stages"
fi

if grep -Eq 'subagent:|fan-in|planning_context|implementation_context|before_review|before_qa|before_execution' "$RESULT_DIR/graph.mmd"; then
  fail "mermaid visible labels should not mix internal English subagent/group names"
fi

grep -q 'audit\[自查\]' "$RESULT_DIR/graph.mmd" || fail "graph should show the two-character audit label"
grep -q 'targeted_repair\[修复\]' "$RESULT_DIR/graph.mmd" || fail "graph should show the two-character repair label"
grep -q 'qa\[测试\]' "$RESULT_DIR/graph.mmd" || fail "graph should show the two-character QA label"
grep -q '环境阻塞' "$RESULT_DIR/graph.mmd" || fail "graph should show recoverable environment blocking"
grep -q '停滞阻塞' "$RESULT_DIR/graph.mmd" || fail "graph should show recoverable stalled blocking"
grep -q '达到3轮，不再自查.*qa' "$RESULT_DIR/graph.mmd" || fail "graph should show the configured audit cap enters QA"

note "contract passed: default quality loop caps audits and graph is compact Chinese"
