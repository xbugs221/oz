#!/usr/bin/env bash
# 文件功能目的：验证 oz flow 删除固定外置子代理编排，只保留主阶段 workflow 与主代理产物门禁。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
CHANGE="42-拆除固定子代理编排"
RESULT_DIR="$ROOT/test-results/42-remove-fixed-subagents"
LOG="$RESULT_DIR/remove-fixed-subagents-contract.log"
GRAPH_EVIDENCE="$RESULT_DIR/default-graph.json"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

mkdir -p "$RESULT_DIR"
: >"$LOG"

note() {
  # note 记录合同测试步骤，便于执行阶段判断失败是否来自目标行为缺失。
  printf '[remove-fixed-subagents] %s\n' "$*" | tee -a "$LOG"
}

fail() {
  # fail 保留明确业务失败原因，避免把语法或环境失败误判为验收失败。
  note "FAIL: $*"
  exit 1
}

assert_file_has() {
  # assert_file_has 证明目标文件仍保留主流程必要入口。
  local file="$1"
  local pattern="$2"
  [[ -f "$file" ]] || fail "缺少文件：$file"
  rg -n "$pattern" "$file" >>"$LOG" || fail "$file 缺少模式：$pattern"
}

assert_file_lacks() {
  # assert_file_lacks 证明目标文件不再暴露外置子代理配置或 prompt 入口。
  local file="$1"
  local pattern="$2"
  [[ -f "$file" ]] || fail "缺少文件：$file"
  if rg -n "$pattern" "$file" >>"$LOG"; then
    fail "$file 仍包含已删除的外置子代理模式：$pattern"
  fi
}

init_repo() {
  # init_repo 创建可运行 oz flow 命令的最小 git 仓库。
  local dir="$1"
  mkdir -p "$dir"
  git -C "$dir" init -q
  git -C "$dir" config user.email "test@example.com"
  git -C "$dir" config user.name "Test User"
}

expect_config_rejected() {
  # expect_config_rejected 验证旧字段不会被静默忽略。
  local name="$1"
  local pattern="$2"
  local project="$TMP/reject-$name"
  init_repo "$project"
  shift 2
  cat >"$project/oz-flow.yaml"
  set +e
  (cd "$project" && "$OZ_BIN" flow graph --change demo --format json) >"$TMP/$name.out" 2>"$TMP/$name.err"
  local code=$?
  set -e
  cat "$TMP/$name.out" "$TMP/$name.err" >>"$LOG"
  [[ "$code" -ne 0 ]] || fail "旧配置 $name 必须被拒绝，不能继续成功加载"
  rg -ni "$pattern" "$TMP/$name.out" "$TMP/$name.err" >>"$LOG" || fail "旧配置 $name 的错误必须提到字段和已删除语义"
}

cd "$ROOT"

note "构建真实 oz 二进制"
OZ_BIN="$TMP/oz"
go build -o "$OZ_BIN" ./cmd/oz 2>&1 | tee -a "$LOG"

note "生成默认 oz-flow.yaml 并验证不再包含外置子代理配置"
PROJECT="$TMP/project"
init_repo "$PROJECT"
(cd "$PROJECT" && "$OZ_BIN" flow config) 2>&1 | tee -a "$LOG"
DEFAULT_CONFIG="$PROJECT/oz-flow.yaml"
assert_file_has "$DEFAULT_CONFIG" '^stages:$'
assert_file_has "$DEFAULT_CONFIG" '^[[:space:]]+execution:$'
assert_file_has "$DEFAULT_CONFIG" '^[[:space:]]+review:$'
assert_file_has "$DEFAULT_CONFIG" '^[[:space:]]+qa:$'
assert_file_has "$DEFAULT_CONFIG" '^[[:space:]]+fix:$'
assert_file_has "$DEFAULT_CONFIG" '^[[:space:]]+archive:$'
assert_file_lacks "$DEFAULT_CONFIG" '(^|[[:space:]])parallel:'
assert_file_lacks "$DEFAULT_CONFIG" '(^|[[:space:]])subagent_guard:'
assert_file_lacks "$DEFAULT_CONFIG" '(^|[[:space:]])before:'
assert_file_lacks "$DEFAULT_CONFIG" '代码库侦察员|目标核对审核员|浏览器路径测试员|安全风险审核员'

note "验证默认 workflow graph 不再包含 subagent/fan-in/parallel artifact"
(cd "$PROJECT" && "$OZ_BIN" flow graph --change demo --format json) >"$GRAPH_EVIDENCE"
python3 - "$GRAPH_EVIDENCE" <<'PY' 2>&1 | tee -a "$LOG"
import json
import sys

path = sys.argv[1]
with open(path, encoding="utf-8") as handle:
    graph = json.load(handle)

nodes = graph.get("nodes", [])
artifacts = graph.get("artifacts", [])
bad_nodes = [node for node in nodes if node.get("type") in {"subagent", "fanin"}]
if bad_nodes:
    raise SystemExit(f"graph must not contain subagent/fanin nodes: {bad_nodes}")

bad_artifacts = [artifact for artifact in artifacts if "parallel" in artifact.get("path", "")]
if bad_artifacts:
    raise SystemExit(f"graph must not contain parallel artifacts: {bad_artifacts}")

node_ids = {node.get("id") for node in nodes}
required = {"execution", "review_1", "qa_1", "fix_1", "archive", "gate_review_1", "gate_qa_1", "gate_archive"}
missing = sorted(required - node_ids)
if missing:
    raise SystemExit(f"graph missing main workflow nodes: {missing}")

print("graph topology assertions passed")
PY

note "验证内置主阶段 prompt 不再读取 oz 子代理 artifact"
assert_file_lacks "prompts-template/oz-flow-start.md" 'subagent artifact|parallel-|ParallelContext|ParallelReview|ParallelQA|helper'
assert_file_lacks "prompts-template/oz-flow-review.md" 'subagent artifact|parallel-|ParallelContext|ParallelReview|ParallelQA|review helper|QA helper'
assert_file_lacks "prompts-template/oz-flow-qa.md" 'subagent artifact|parallel-|ParallelContext|ParallelReview|ParallelQA|review helper|QA helper'
assert_file_has "prompts-template/oz-flow-start.md" 'StatePath|AcceptancePath|ChangePath'
assert_file_has "prompts-template/oz-flow-review.md" 'StatePath|AcceptancePath|ChangePath|ReviewPath'
assert_file_has "prompts-template/oz-flow-qa.md" 'StatePath|AcceptancePath|ChangePath|ReviewPath|QAPath'

note "验证旧外置子代理配置字段明确拒绝"
expect_config_rejected "parallel" 'parallel.*(已删除|不再支持|removed|unsupported)' <<'YAML'
parallel: true
stages:
  execution:
    agent: codex
YAML
expect_config_rejected "parallel-empty" 'parallel.*(已删除|不再支持|removed|unsupported)' <<'YAML'
parallel: {}
stages:
  execution:
    agent: codex
YAML
expect_config_rejected "parallel-null" 'parallel.*(已删除|不再支持|removed|unsupported)' <<'YAML'
parallel:
stages:
  execution:
    agent: codex
YAML
expect_config_rejected "subagents-empty" 'subagents.*(已删除|不再支持|removed|unsupported)' <<'YAML'
subagents: {}
stages:
  execution:
    agent: codex
YAML
expect_config_rejected "subagent-guard" 'subagent_guard.*(已删除|不再支持|removed|unsupported)' <<'YAML'
subagent_guard: advisory
stages:
  execution:
    agent: codex
YAML
expect_config_rejected "subagent-guard-null" 'subagent_guard.*(已删除|不再支持|removed|unsupported)' <<'YAML'
subagent_guard:
stages:
  execution:
    agent: codex
YAML
expect_config_rejected "before" 'before.*(已删除|不再支持|removed|unsupported)' <<'YAML'
stages:
  execution:
    agent: codex
    before:
      - name: 代码库侦察员
        purpose: 搜索 execution 需要读取的文件和测试模式
        agent: pi
YAML

note "验证生产代码不再保留外置子代理 runner/fan-in 边界"
if rg -n 'nodeRunSubagent|nodeFanin|runSubagentAttempts|ParallelMemberResult|memberArtifactPath|ValidateParallelQAGate' internal/app >>"$LOG"; then
  fail "internal/app 仍包含 oz 外置子代理 runner、fan-in、member artifact 或 parallel gate 边界"
fi

note "运行主流程相关 Go 回归"
go test ./internal/app ./internal/ozcli ./tests -count=1 2>&1 | tee -a "$LOG"

note "PASS: $CHANGE"
