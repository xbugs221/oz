#!/usr/bin/env bash
# 文件目的：验证 task 未完成时，wo 仍会启动 execution 前的代码侦察和外部资料 subagents。
# Sources: 17-已完成执行跳过上下文subagents
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

log_dir="$repo_root/test-results/17-skip-execution-context"
mkdir -p "$log_dir"
log="$log_dir/pending-runs-context.log"
state_evidence="$log_dir/pending-runs-context-state.json"
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
subagent_marker="$tmp/subagents-called.txt"

cat >"$fakebin/codex" <<'SH'
#!/usr/bin/env bash
# 文件目的：模拟 subagent、execution 和 archive 的最小真实副作用。
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
name = re.search(r"SUBAGENT_NAME=(.+)", prompt)
if name:
    purpose = re.search(r"SUBAGENT_PURPOSE=(.+)", prompt)
    change_name = re.search(r"CURRENT_CHANGE=(.+)", prompt)
    member_name = name.group(1).strip() if name else "并行成员"
    body = {
        "name": member_name,
        "change_name": change_name.group(1).strip() if change_name else "17-pending",
        "purpose": purpose.group(1).strip() if purpose else "执行前上下文",
        "status": "success",
        "summary": member_name + " 已提供 execution 前上下文",
        "evidence": ["test-results/17-skip-execution-context/pending-runs-context.log"]
    }
    with pathlib.Path(os.environ["SUBAGENT_MARKER"]).open("a", encoding="utf-8") as handle:
        handle.write(member_name + "\n")
    print(json.dumps({"type": "session", "id": "fake-subagent-" + member_name}))
    print(json.dumps({"type": "thread.started", "thread_id": "fake-subagent-" + member_name}))
    print(json.dumps({"type": "message", "message": {"role": "assistant", "content": [{"type": "text", "text": json.dumps(body, ensure_ascii=False)}]}}, ensure_ascii=False))
    raise SystemExit(0)

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
    task = repo / "docs" / "changes" / change / "task.md"
    text = task.read_text(encoding="utf-8")
    if "- [ ]" not in text:
        raise SystemExit("pending scenario expected an unchecked task before execution")
    task.write_text(text.replace("- [ ]", "- [x]"), encoding="utf-8")
    evidence = repo / "test-results" / "temporary.log"
    evidence.parent.mkdir(parents=True, exist_ok=True)
    evidence.write_text("execution evidence\n", encoding="utf-8")
    print(json.dumps({"type": "session", "id": "fake-execution-session"}))
    print(json.dumps({"type": "thread.started", "thread_id": "fake-execution-session"}))
    raise SystemExit(0)

if stage == "archive":
    archive = repo / "docs" / "changes" / "archive" / ("2026-06-11-" + change)
    archive.mkdir(parents=True, exist_ok=True)
    source = repo / "docs" / "changes" / change / "acceptance.json"
    (archive / "acceptance.json").write_text(source.read_text(encoding="utf-8"), encoding="utf-8")
    (run_dir / "delivery-summary.md").write_text("task 未完成时先运行 execution context subagents，再归档。\n", encoding="utf-8")
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
change="1-未完成执行保留上下文"
mkdir -p "$project/docs/changes/$change/tests"
git -C "$project" init -q
git -C "$project" config user.email "test@example.com"
git -C "$project" config user.name "Test User"

cat >"$project/docs/changes/$change/brief.md" <<'MD'
# 未完成执行保留上下文

这个临时 change 用于验证 task 未完成时仍应启动 execution 上下文 subagents。
MD

cat >"$project/docs/changes/$change/proposal.md" <<'MD'
# 未完成执行保留上下文

## 背景

task 尚未完成，workflow 需要执行前上下文辅助主 execution。
MD

cat >"$project/docs/changes/$change/design.md" <<'MD'
# 设计

使用未勾选 task 表达执行阶段仍需运行。
MD

cat >"$project/docs/changes/$change/spec.md" <<'MD'
# 规格

### 需求：保留未完成执行上下文

系统必须在 task 未完成时运行 execution context subagents。

#### 场景：task 未完成

- **当** 用户运行 wo run
- **则** 先启动 execution context subagents
MD

cat >"$project/docs/changes/$change/task.md" <<'MD'
# 任务

- [ ] 1.1 尚未完成执行任务
MD

cat >"$project/docs/changes/$change/tests/test_contract.sh" <<'SH'
#!/usr/bin/env bash
# 文件目的：临时 change 的真实测试入口，证明 execution 会把 task 勾选完成。
set -euo pipefail
grep -qF "[x]" docs/changes/1-未完成执行保留上下文/task.md
SH
chmod +x "$project/docs/changes/$change/tests/test_contract.sh"

cat >"$project/docs/changes/$change/acceptance.json" <<'JSON'
{
  "summary": "验证未完成 task 仍需要 execution context subagents",
  "required_tests": [
    {
      "id": "temporary-contract",
      "source": "change_contract",
      "path": "docs/changes/1-未完成执行保留上下文/tests/test_contract.sh",
      "command": "bash docs/changes/1-未完成执行保留上下文/tests/test_contract.sh",
      "purpose": "证明临时 change task 由 execution 完成",
      "assertions": ["task.md 中的执行任务从未勾选变为已勾选，workflow 保留 execution 前上下文"]
    }
  ],
  "required_evidence": []
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

note "运行 wo run：task 未完成时必须保留 execution context subagents"
WO_TEST_REPO="$project" \
SUBAGENT_MARKER="$subagent_marker" \
XDG_STATE_HOME="$tmp/state" \
HOME="$tmp/home" \
PATH="$fakebin:/usr/bin:/bin" \
  bash -c 'cd "$1" && "$2" run --change "$3" --json' _ "$project" "$wo" "$change" >"$tmp/run.jsonl" 2>"$tmp/run.err" || {
    cat "$tmp/run.err" | tee -a "$log"
    fail "未完成 task 场景必须能运行 execution context subagents 并完成 workflow"
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
    raise SystemExit(f"workflow should finish after pending execution, got {state.get('status')}/{state.get('stage')}")

called = marker.read_text(encoding="utf-8").splitlines() if marker.exists() else []
for name in ["代码侦察", "外部资料"]:
    if name not in called:
        raise SystemExit(f"expected execution context subagent {name} to run, called={called}")

fanin = run_dir / "parallel-implementation-context.json"
if not fanin.exists():
    raise SystemExit("parallel-implementation-context.json should exist for pending task execution")
artifact = json.loads(fanin.read_text(encoding="utf-8"))
members = artifact.get("members", [])
if len(members) != 2:
    raise SystemExit(f"fan-in artifact should contain 2 members, got {len(members)}")
if any(member.get("status") != "success" for member in members):
    raise SystemExit(f"all subagent members should succeed: {members}")

sessions = state.get("sessions", {})
subagent_sessions = [key for key in sessions if "subagent:implementation_context" in key]
if len(subagent_sessions) != 2:
    raise SystemExit(f"pending task should record 2 implementation_context subagent sessions, got {subagent_sessions}")
PY

note "PASS: task 未完成时 execution context subagents 正常运行"
