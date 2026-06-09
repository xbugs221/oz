#!/usr/bin/env bash
# 文件功能目的：验证 wo 能将 OmO parallel subagent 工作流导出为 Dagu YAML、Mermaid 和 JSON。
# Sources: 29-使用Dagu编排和可视化OmO工作流
set -euo pipefail

ROOT=$(git rev-parse --show-toplevel)
WORK=$(mktemp -d)
RESULT_DIR="$ROOT/test-results/specs/codex-workflow-cli/dagu-omo"
trap 'rm -rf "$WORK"' EXIT

mkdir -p "$WORK/repo" "$RESULT_DIR"
rm -f "$RESULT_DIR/contract.log"
(cd "$ROOT" && tar --exclude='.git' --exclude='test-results' -cf - .) | (cd "$WORK/repo" && tar -xf -)
cd "$WORK/repo"
git init -q

cat > wo.yaml <<'YAML'
wo:
  workflow:
    max_review_iterations: 2
    stages:
      planning:
        cli: codex
      execution:
        cli: pi
      review:
        cli: codex
      qa:
        cli: pi
      fix:
        cli: pi
      archive:
        cli: codex
    subagents:
      enabled: true
      groups:
        before_execution:
          mode: advisory
          members:
            - name: 代码库侦察员
              cli: pi
              purpose: 搜索相关模块和测试入口
            - name: 外部资料研究员
              cli: codex
              purpose: 查询依赖库文档
        before_review:
          mode: gate_input
          members:
            - name: 目标核对审核员
              cli: codex
              purpose: 核对 proposal/spec/task 是否满足
            - name: 安全风险审核员
              cli: codex
              purpose: 检查权限、输入和泄漏风险
        before_qa:
          mode: gate_input
          members:
            - name: CLI/API 测试员
              cli: pi
              purpose: 执行命令行或接口真实路径
            - name: 回归场景测试员
              cli: pi
              purpose: 覆盖邻近功能回归
YAML

mkdir -p docs/changes/demo
cat > docs/changes/demo/acceptance.json <<'JSON'
{
  "summary": "demo acceptance",
  "required_tests": [],
  "required_evidence": []
}
JSON

exec > >(tee "$RESULT_DIR/contract.log") 2>&1

echo "running: go run ./cmd/wo graph --change demo --format dagu"
go run ./cmd/wo graph --change demo --format dagu > "$RESULT_DIR/workflow.dagu.yaml"
echo "running: go run ./cmd/wo graph --change demo --format mermaid"
go run ./cmd/wo graph --change demo --format mermaid > "$RESULT_DIR/workflow.mmd"
echo "running: go run ./cmd/wo graph --change demo --format json"
go run ./cmd/wo graph --change demo --format json > "$RESULT_DIR/workflow-spec.json"
mkdir -p sub
echo "running: cd sub && go run ../cmd/wo graph --change demo --format json"
(cd sub && go run ../cmd/wo graph --change demo --format json) > "$RESULT_DIR/workflow-spec-from-subdir.json"

echo "asserting: WorkflowSpec contains graph node types and fan-out dependencies"
python3 - "$RESULT_DIR/workflow-spec.json" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, "r", encoding="utf-8") as fh:
    data = json.load(fh)
nodes = data.get("nodes", [])
types = {node.get("type") for node in nodes}
required_types = {"main_stage", "subagent", "fanin", "gate"}
missing = required_types - types
if missing:
    raise SystemExit(f"missing node types: {sorted(missing)}")
names = " ".join(str(node.get("id", "")) + " " + str(node.get("name", "")) for node in nodes)
for text in ["execution", "review_1", "qa_1", "fix_1", "archive", "before_execution", "before_review", "before_qa"]:
    if text not in names:
        raise SystemExit(f"missing node text: {text}")
edges = {(edge.get("from"), edge.get("to")) for edge in data.get("edges", [])}
required_edges = {
    ("before_execution_1", "before_execution_fanin"),
    ("before_execution_2", "before_execution_fanin"),
    ("execution", "before_review_1_1"),
    ("execution", "before_review_1_2"),
    ("before_review_1_1", "before_review_1_fanin"),
    ("before_review_1_2", "before_review_1_fanin"),
    ("before_review_1_fanin", "review_1"),
    ("gate_review_1", "before_qa_1_1"),
    ("gate_review_1", "before_qa_1_2"),
    ("before_qa_1_1", "before_qa_1_fanin"),
    ("before_qa_1_2", "before_qa_1_fanin"),
    ("before_qa_1_fanin", "qa_1"),
    ("fix_1", "before_review_2_1"),
    ("fix_1", "before_review_2_2"),
}
missing_edges = required_edges - edges
if missing_edges:
    raise SystemExit(f"missing workflow edges: {sorted(missing_edges)}")
for forbidden_edge in [("execution", "before_review_1_fanin"), ("gate_review_1", "before_qa_1_fanin")]:
    if forbidden_edge in edges:
        raise SystemExit(f"fanin must not depend directly on stage gate: {forbidden_edge}")
PY

echo "asserting: graph reads repository wo.yaml when launched from a subdirectory"
python3 - "$RESULT_DIR/workflow-spec.json" "$RESULT_DIR/workflow-spec-from-subdir.json" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as fh:
    root = json.load(fh)
with open(sys.argv[2], "r", encoding="utf-8") as fh:
    subdir = json.load(fh)
root_subagents = [node for node in root.get("nodes", []) if node.get("type") == "subagent"]
subdir_subagents = [node for node in subdir.get("nodes", []) if node.get("type") == "subagent"]
if len(root_subagents) != len(subdir_subagents):
    raise SystemExit(f"subdir graph ignored repository config: root={len(root_subagents)} subdir={len(subdir_subagents)}")
if not subdir_subagents:
    raise SystemExit("subdir graph did not load configured subagents")
root_edges = {(edge.get("from"), edge.get("to")) for edge in root.get("edges", [])}
subdir_edges = {(edge.get("from"), edge.get("to")) for edge in subdir.get("edges", [])}
if root_edges != subdir_edges:
    raise SystemExit("subdir graph edges differ from repository-root graph")
PY

echo "asserting: Dagu YAML contains stable wo node commands"
for text in \
  "name:" \
  "before_execution" \
  "before_review" \
  "before_qa" \
  "run-subagent" \
  "node fanin" \
  "node run-stage" \
  "node gate"; do
  if ! grep -qF "$text" "$RESULT_DIR/workflow.dagu.yaml"; then
    echo "Dagu YAML missing: $text" >&2
    cat "$RESULT_DIR/workflow.dagu.yaml" >&2
    exit 1
  fi
done

echo "asserting: Dagu YAML does not directly call backend CLIs"
for forbidden in "codex exec" "pi --mode json" "opencode run"; do
  if grep -qF "$forbidden" "$RESULT_DIR/workflow.dagu.yaml"; then
    echo "Dagu YAML must not directly call backend: $forbidden" >&2
    cat "$RESULT_DIR/workflow.dagu.yaml" >&2
    exit 1
  fi
done

echo "asserting: Mermaid graph contains business gates and branches"
for text in \
  "flowchart" \
  "before_execution" \
  "before_review" \
  "before_qa" \
  "fan-in" \
  "review clean" \
  "review needs_fix" \
  "QA clean" \
  "QA needs_fix" \
  "archive gate"; do
  if ! grep -qF "$text" "$RESULT_DIR/workflow.mmd"; then
    echo "Mermaid graph missing: $text" >&2
    cat "$RESULT_DIR/workflow.mmd" >&2
    exit 1
  fi
done

echo "contract passed"
