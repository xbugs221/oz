#!/usr/bin/env bash
# 文件目的：验证 task 已全部完成时，wo 不再启动 execution 前的代码侦察和外部资料 subagents。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

log_dir="$repo_root/test-results/17-skip-execution-context"
mkdir -p "$log_dir"
log="$log_dir/skip-when-done.log"
state_evidence="$log_dir/skip-when-done-state.json"
: >"$log"

note() {
  # note 同时写入终端和 runtime log，方便 QA 复核失败位置。
  printf '%s\n' "$*" | tee -a "$log"
}

fail() {
  # fail 用明确业务原因终止测试，避免静默误判。
  note "FAIL: $*"
  exit 1
}

cd "$repo_root"
wo="$tmp/wo"
oz="$tmp/oz"
go build -o "$wo" ./cmd/wo 2>&1 | tee -a "$log"
go build -o "$oz" ./cmd/oz 2>&1 | tee -a "$log"

fakebin="$tmp/fakebin"
mkdir -p "$fakebin"
ln -s "$oz" "$fakebin/oz"
subagent_marker="$tmp/unexpected-subagent.txt"

cat >"$fakebin/codex" <<'SH'
#!/usr/bin/env bash
# 文件目的：模拟主 agent；如果收到 subagent prompt，说明 workflow 错误消耗了执行前上下文资源。
set -euo pipefail

prompt="$(cat)"
if [ -z "$prompt" ] && [ "$#" -gt 0 ]; then
  prompt="${!#}"
fi
python3 - "$prompt" <<'PY'
import json
import os
import pathlib
import re
import sys

prompt = sys.argv[1]
if re.search(r"SUBAGENT_OUTPUT=", prompt):
    marker = pathlib.Path(os.environ["SUBAGENT_MARKER"])
    marker.write_text("unexpected execution-context subagent was invoked\n", encoding="utf-8")
    raise SystemExit("execution context subagent should be skipped when all tasks are done")

state_home = pathlib.Path(os.environ["XDG_STATE_HOME"])
states = sorted(state_home.glob("wo/repos/*/runs/*/state.json"))
if not states:
    raise SystemExit("no wo state.json found")
state_path = states[-1]
state = json.loads(state_path.read_text(encoding="utf-8"))
repo = pathlib.Path(os.environ["WO_TEST_REPO"])
run_dir = state_path.parent
change = state["change_name"]
stage = state["stage"]

if stage == "execution":
    raise SystemExit("execution main agent should not run when all tasks are already done")

if stage == "archive":
    archive = repo / "docs" / "changes" / "archive" / ("2026-06-11-" + change)
    archive.mkdir(parents=True, exist_ok=True)
    source = repo / "docs" / "changes" / change / "acceptance.json"
    (archive / "acceptance.json").write_text(source.read_text(encoding="utf-8"), encoding="utf-8")
    (run_dir / "delivery-summary.md").write_text("task 已完成，跳过 execution context subagents 后归档。\n", encoding="utf-8")
    print(json.dumps({"type": "session", "id": "fake-archive-session"}))
    print(json.dumps({"type": "thread.started", "thread_id": "fake-archive-session"}))
    raise SystemExit(0)

raise SystemExit(f"unexpected main stage: {stage}")
PY
SH
chmod +x "$fakebin/codex"
cp "$fakebin/codex" "$fakebin/pi"
cp "$fakebin/codex" "$fakebin/agy"

project="$tmp/project"
change="1-已完成执行跳过上下文"
mkdir -p "$project/docs/changes/$change/tests"
git -C "$project" init -q
git -C "$project" config user.email "test@example.com"
git -C "$project" config user.name "Test User"

cat >"$project/docs/changes/$change/brief.md" <<'MD'
# 已完成执行跳过上下文

这个临时 change 用于验证 task 已完成时不应启动 execution 上下文 subagents。
MD

cat >"$project/docs/changes/$change/proposal.md" <<'MD'
# 已完成执行跳过上下文

## 背景

task 已经完成，workflow 只需要继续验收和归档。
MD

cat >"$project/docs/changes/$change/design.md" <<'MD'
# 设计

使用已勾选 task 表达执行阶段无需再次运行。
MD

cat >"$project/docs/changes/$change/spec.md" <<'MD'
# 规格

### 需求：跳过已完成执行上下文

系统必须跳过已完成 task 的 execution context subagents。

#### 场景：task 已完成

- **当** 用户运行 wo run
- **则** 不启动 execution context subagents
MD

cat >"$project/docs/changes/$change/task.md" <<'MD'
# 任务

- [x] 1.1 已经完成执行任务
MD

cat >"$project/docs/changes/$change/tests/test_contract.sh" <<'SH'
#!/usr/bin/env bash
# 文件目的：临时 change 的真实测试入口，证明 acceptance 测试不是空目录。
set -euo pipefail
grep -qF "[x]" docs/changes/1-已完成执行跳过上下文/task.md
SH
chmod +x "$project/docs/changes/$change/tests/test_contract.sh"

cat >"$project/docs/changes/$change/acceptance.json" <<'JSON'
{
  "summary": "验证已完成 task 不需要 execution context subagents",
  "required_tests": [
    {
      "id": "temporary-contract",
      "source": "change_contract",
      "path": "docs/changes/1-已完成执行跳过上下文/tests/test_contract.sh",
      "command": "bash docs/changes/1-已完成执行跳过上下文/tests/test_contract.sh",
      "purpose": "证明临时 change task 已完成",
      "assertions": ["task.md 中的执行任务已经勾选，workflow 不需要再次启动 execution agent"]
    }
  ],
  "required_evidence": [
    {
      "id": "temporary-log",
      "kind": "runtime_log",
      "path": "test-results/temporary.log",
      "purpose": "记录临时 workflow 运行"
    }
  ]
}
JSON

cat >"$project/wo.yaml" <<'YAML'
wo:
  workflow:
    engine: go-dag
    max_review_iterations: 0
    stages:
      execution:
        cli: codex
        reasoning: medium
        permissions: default
      archive:
        cli: codex
        reasoning: low
        permissions: default
    parallel:
      enabled: true
      groups:
        implementation_context:
          mode: advisory
          members:
            - name: 代码侦察
              purpose: 汇总 execution 需要读取的文件和测试模式
              stage: before_execution
              tool: pi
              subagent: explore
            - name: 外部资料
              purpose: 查询 execution 依赖的外部库文档和兼容性要求
              stage: before_execution
              tool: pi
              subagent: librarian
    validation:
      max_attempts_per_stage: 3
      commands: []
YAML

git -C "$project" add .
git -C "$project" commit -q -m initial

note "运行 wo run：task 已完成时不应启动 execution context subagents"
WO_TEST_REPO="$project" \
SUBAGENT_MARKER="$subagent_marker" \
XDG_STATE_HOME="$tmp/state" \
HOME="$tmp/home" \
PATH="$fakebin:/usr/bin:/bin" \
  bash -c 'cd "$1" && "$2" run --change "$3" --json' _ "$project" "$wo" "$change" >"$tmp/run.jsonl" 2>"$tmp/run.err" || {
    cat "$tmp/run.err" | tee -a "$log"
    fail "已完成 task 场景不应因为 execution context subagent 被调用而失败"
  }

cat "$tmp/run.jsonl" >>"$log"

state_path="$(XDG_STATE_HOME="$tmp/state" python3 - <<'PY'
import os
import pathlib

states = sorted(pathlib.Path(os.environ["XDG_STATE_HOME"]).glob("wo/repos/*/runs/*/state.json"))
if not states:
    raise SystemExit("no state.json found")
print(states[-1])
PY
)"
cp "$state_path" "$state_evidence"

python3 - "$state_path" "$subagent_marker" <<'PY'
import json
import pathlib
import sys

state_path = pathlib.Path(sys.argv[1])
marker = pathlib.Path(sys.argv[2])
state = json.loads(state_path.read_text(encoding="utf-8"))
run_dir = state_path.parent

if state.get("status") != "done" or state.get("stage") != "done":
    raise SystemExit(f"workflow should finish after skipping execution context, got {state.get('status')}/{state.get('stage')}")

sessions = state.get("sessions", {})
subagent_sessions = [key for key in sessions if "subagent:" in key]
if subagent_sessions:
    raise SystemExit(f"execution context subagent sessions should not exist: {subagent_sessions}")

if marker.exists() and marker.read_text(encoding="utf-8").strip():
    raise SystemExit(marker.read_text(encoding="utf-8"))

parallel_members = run_dir / "parallel-members"
if parallel_members.exists() and any(parallel_members.rglob("*.json")):
    raise SystemExit("parallel member artifacts should not be created when task is already done")

for name in ["parallel-implementation-context.json", "parallel-planning-context.json"]:
    if (run_dir / name).exists():
        raise SystemExit(f"{name} should not be required or generated for already-done execution")
PY

note "PASS: task 已完成时 execution context subagents 没有被调用"
