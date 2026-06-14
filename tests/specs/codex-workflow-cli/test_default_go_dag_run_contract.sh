#!/usr/bin/env bash
# 文件目的：通过真实 oz flow run/status 入口验证默认执行使用内嵌 go-dag engine。
# Sources: 3-默认启用-纯go-dag并行subagents
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

log_dir="$repo_root/test-results/go-dag"
mkdir -p "$log_dir"
log="$log_dir/default-go-dag-run-contract.log"
: >"$log"

# note 把关键验证步骤同时写到 stdout 和 runtime log，方便 QA 复查失败点。
note() {
  printf '%s\n' "$*" | tee -a "$log"
}

# fail 用统一格式终止测试，避免静默通过。
fail() {
  note "FAIL: $*"
  exit 1
}

cd "$repo_root"
wo="$tmp/wo"
oz="$tmp/oz"
go build -o "$wo" ./cmd/oz 2>&1 | tee -a "$log"
go build -o "$oz" ./cmd/oz 2>&1 | tee -a "$log"

fakebin="$tmp/fakebin"
mkdir -p "$fakebin"
ln -s "$oz" "$fakebin/oz"

cat >"$fakebin/codex" <<'SH'
#!/usr/bin/env bash
# 这个 fake codex 只做本测试需要的最小真实副作用：写 subagent artifact、勾选任务、写归档摘要。
set -euo pipefail

prompt="$(cat)"
args="$*"
if [ -n "$args" ]; then
  prompt="$prompt
$args"
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
    body = {
        "name": name.group(1).strip(),
        "change_name": change_name.group(1).strip() if change_name else "demo",
        "purpose": purpose.group(1).strip() if purpose else "执行并行成员职责",
        "status": "success",
        "summary": "fake agent completed the configured read-only subagent task",
        "evidence": ["test-results/go-dag/default-go-dag-run-contract.log"]
    }
    print(json.dumps({"type": "thread.started", "thread_id": "fake-subagent-session"}))
    print(json.dumps({"type": "message", "message": {"role": "assistant", "content": [{"type": "text", "text": json.dumps(body, ensure_ascii=False)}]}}, ensure_ascii=False))
    raise SystemExit(0)

state_home = pathlib.Path(os.environ["XDG_STATE_HOME"])
states = sorted(state_home.glob("oz/flow/repos/*/runs/*/state.json"))
if not states:
    raise SystemExit("no oz flow state.json found")
state_path = states[-1]
state = json.loads(state_path.read_text(encoding="utf-8"))
repo = pathlib.Path(os.environ["WO_TEST_REPO"])
run_dir = state_path.parent
change = state["change_name"]
stage = state["stage"]

if stage == "execution":
    task = repo / "docs" / "changes" / change / "task.md"
    text = task.read_text(encoding="utf-8")
    task.write_text(text.replace("- [ ]", "- [x]"), encoding="utf-8")
    evidence = repo / "test-results" / "go-dag" / "temporary.log"
    evidence.parent.mkdir(parents=True, exist_ok=True)
    evidence.write_text("execution evidence\n", encoding="utf-8")
elif stage == "archive":
    archive = repo / "docs" / "changes" / "archive" / ("2026-06-09-" + change)
    archive.mkdir(parents=True, exist_ok=True)
    (archive / "acceptance.json").write_text((repo / "docs" / "changes" / change / "acceptance.json").read_text(encoding="utf-8"), encoding="utf-8")
    (run_dir / "delivery-summary.md").write_text("fake go-dag archive completed\n", encoding="utf-8")

print(json.dumps({"type": "thread.started", "thread_id": "fake-main-session-" + stage}))
PY
SH
chmod +x "$fakebin/codex"

cp "$fakebin/codex" "$fakebin/legacy-agent"
cp "$fakebin/codex" "$fakebin/pi"
cp "$fakebin/codex" "$fakebin/agy"

project="$tmp/project"
mkdir -p "$project/docs/changes/1-默认go-dag/tests"
git -C "$project" init -q
git -C "$project" config user.email "test@example.com"
git -C "$project" config user.name "Test User"

cat >"$project/docs/changes/1-默认go-dag/proposal.md" <<'MD'
# 默认 go-dag

## 背景

这个临时 change 用来验证 oz flow 默认运行路径。
MD

cat >"$project/docs/changes/1-默认go-dag/brief.md" <<'MD'
# 默认 go-dag

验证默认 oz flow run 使用 go-dag 推进真实 change，并保留状态观测合同。
MD

cat >"$project/docs/changes/1-默认go-dag/design.md" <<'MD'
# 设计

使用最小任务证明默认 go-dag 运行会推进真实 change。
MD

cat >"$project/docs/changes/1-默认go-dag/spec.md" <<'MD'
# 规格

### 需求：默认 go-dag

系统必须默认使用 go-dag。

#### 场景：运行并归档

- **当** 用户运行 oz flow run
- **则** run 完成并记录 go-dag 状态
MD

cat >"$project/docs/changes/1-默认go-dag/task.md" <<'MD'
# 任务

- [ ] 1.1 完成默认 go-dag 验证任务
MD

cat >"$project/docs/changes/1-默认go-dag/tests/test_contract.sh" <<'SH'
#!/usr/bin/env bash
# 临时 change 的测试入口，证明 tests/ 目录不是占位。
set -euo pipefail
test -f docs/changes/1-默认go-dag/acceptance.json
SH
chmod +x "$project/docs/changes/1-默认go-dag/tests/test_contract.sh"

cat >"$project/docs/changes/1-默认go-dag/acceptance.json" <<'JSON'
{
  "summary": "验证默认 go-dag run 契约",
  "required_tests": [
    {
      "id": "temporary-contract",
      "source": "change_contract",
      "path": "docs/changes/1-默认go-dag/tests/test_contract.sh",
      "command": "bash docs/changes/1-默认go-dag/tests/test_contract.sh",
      "purpose": "证明临时 change 包含真实测试入口",
      "assertions": ["默认 oz flow run 使用 go-dag 推进 execution 并完成 archive"]
    }
  ],
  "required_evidence": []
}
JSON

cat >"$project/oz-flow.yaml" <<'YAML'
wo:
  workflow:
    max_review_iterations: 0
YAML

git -C "$project" add .
git -C "$project" commit -q -m initial

note "运行默认 oz flow run，验证内嵌 go-dag 能推进真实 change"
WO_TEST_REPO="$project" \
XDG_STATE_HOME="$tmp/state" \
HOME="$tmp/home" \
PATH="$fakebin:/usr/bin:/bin" \
  bash -c 'cd "$1" && "$2" run --change "1-默认go-dag" --json' _ "$project" "$wo" >"$tmp/run.jsonl" 2>"$tmp/run.err" || {
    cat "$tmp/run.err" | tee -a "$log"
    fail "默认 oz flow run 失败"
  }

cat "$tmp/run.jsonl" >>"$log"

state="$(find "$tmp/state/oz/flow/repos" -path '*/runs/*/state.json' -print | sort | tail -n 1)"
test -n "$state" || fail "缺少 run state.json"
note "state: $state"
cat "$state" >>"$log"

python3 - "$state" <<'PY' || exit 1
import json
import sys
state = json.load(open(sys.argv[1], encoding="utf-8"))
if state.get("engine") != "go-dag":
    raise SystemExit("state.engine must be go-dag")
if state.get("workflow_config", {}).get("parallel", {}).get("enabled") is not True:
    raise SystemExit("workflow_config.parallel.enabled must default to true")
PY

run_id="$(python3 - "$state" <<'PY'
import json, sys
print(json.load(open(sys.argv[1], encoding="utf-8"))["run_id"])
PY
)"

note "检查人类 status 输出包含并行成员阶段树"
WO_TEST_REPO="$project" \
XDG_STATE_HOME="$tmp/state" \
HOME="$tmp/home" \
PATH="$fakebin:/usr/bin:/bin" \
  bash -c 'cd "$1" && "$2" status -w1' _ "$project" "$wo" >"$tmp/status.txt"
cat "$tmp/status.txt" >>"$log"
grep -qF "fake-main-session-execution" "$tmp/status.txt" || fail "oz flow status 必须显示 execution 主阶段 session"
grep -qF "代码" "$tmp/status.txt" || fail "oz flow status 必须显示 implementation_context 并行成员"
grep -qF "外部" "$tmp/status.txt" || fail "oz flow status 必须显示 implementation_context 并行成员"

note "检查 JSON status 兼容旧 runner contract"
WO_TEST_REPO="$project" \
XDG_STATE_HOME="$tmp/state" \
HOME="$tmp/home" \
PATH="$fakebin:/usr/bin:/bin" \
  bash -c 'cd "$1" && "$2" status --run-id "$3" --json' _ "$project" "$wo" "$run_id" >"$tmp/status.json"
cat "$tmp/status.json" >>"$log"
python3 - "$tmp/status.json" <<'PY' || exit 1
import json
import sys
payload = json.load(open(sys.argv[1], encoding="utf-8"))
for key in ["run_id", "change_name", "status", "stage", "stages", "paths", "sessions", "error"]:
    if key not in payload:
        raise SystemExit(f"missing JSON key {key}")
for forbidden in ["parallel", "parallel_status", "parallel_summary", "members"]:
    if forbidden in payload:
        raise SystemExit(f"JSON status must not expose {forbidden}")
PY

note "PASS"
