#!/usr/bin/env bash
# 文件功能目的：验证树状 stages.before 子代理默认启动、无关职责输出 relevant:false 不阻断主阶段，并验证主阶段/子代理 model 透传。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
result_dir="$repo_root/test-results/codex-workflow-cli/tree-config"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$result_dir"
log="$result_dir/subagent-relevance.log"
state_evidence="$result_dir/subagent-relevance-state.json"
: >"$log"

note() {
  # note 生成 subagent-relevance-log 证据。
  printf '%s\n' "$*" | tee -a "$log"
}

fail() {
  # fail 输出业务语义，区分配置解析、模型透传和 relevance artifact 问题。
  note "FAIL: $*"
  exit 1
}

wo_bin="$tmp/wo"
oz_bin="$tmp/oz"
note "构建真实 wo/oz binary"
go build -C "$repo_root" -o "$wo_bin" ./cmd/oz 2>&1 | tee -a "$log"
go build -C "$repo_root" -o "$oz_bin" ./cmd/oz 2>&1 | tee -a "$log"

fakebin="$tmp/fakebin"
mkdir -p "$fakebin"
ln -s "$oz_bin" "$fakebin/oz"
agent_log="$tmp/agent-calls.log"
: >"$agent_log"

cat >"$fakebin/codex" <<'SH'
#!/usr/bin/env bash
# 文件功能目的：模拟主阶段 agent，记录 argv 并写出最小有效 execution/review/qa/archive artifact。
set -euo pipefail

printf 'codex argv: %s\n' "$*" >>"${AGENT_CALL_LOG:?}"
prompt="$(cat || true)"

python3 - "$prompt" <<'PY'
import json
import os
import pathlib
import sys

_prompt = sys.argv[1]
state_home = pathlib.Path(os.environ["XDG_STATE_HOME"])
states = sorted(state_home.glob("oz/flow/repos/*/runs/*/state.json"))
if not states:
    raise SystemExit("no state.json found for fake codex")
state_path = states[-1]
state = json.loads(state_path.read_text(encoding="utf-8"))
repo = pathlib.Path(os.environ["WO_TEST_REPO"])
run_dir = state_path.parent
change = state["change_name"]
stage = state["stage"]

if stage == "execution":
    task = repo / "docs" / "changes" / change / "task.md"
    task.write_text(task.read_text(encoding="utf-8").replace("- [ ]", "- [x]"), encoding="utf-8")
elif stage == "review_1":
    (run_dir / "review-1.json").write_text(json.dumps({
        "summary": "CLI 配置变更 review clean",
        "decision": "clean",
        "checks": {
            "oz_aligned": True,
            "tasks_verified": True,
            "tests_meaningful": True,
            "implementation_scoped": True,
            "runtime_behavior_verified": True,
            "previous_findings_resolved": True
        },
        "evidence": [
            "go test runtime evidence: test-results/codex-workflow-cli/tree-config/subagent-relevance.log",
            "qa runtime evidence will be written by fake QA"
        ],
        "findings": []
    }, ensure_ascii=False), encoding="utf-8")
elif stage == "qa_1":
    (run_dir / "qa-1.json").write_text(json.dumps({
        "summary": "CLI 配置变更 QA clean，浏览器路径已判定无关",
        "decision": "clean",
        "evidence": ["runtime log: test-results/codex-workflow-cli/tree-config/subagent-relevance.log"],
        "findings": [],
        "acceptance_matrix": [
            {
                "id": "cli-config-contract",
                "status": "passed",
                "artifact": "docs/changes/1-纯CLI配置变更/tests/test_contract.sh",
                "evidence": "test-results/codex-workflow-cli/tree-config/subagent-relevance.log"
            }
        ]
    }, ensure_ascii=False), encoding="utf-8")
elif stage == "archive":
    archive = repo / "docs" / "changes" / "archive" / ("2026-06-14-" + change)
    archive.mkdir(parents=True, exist_ok=True)
    (archive / "acceptance.json").write_text((repo / "docs" / "changes" / change / "acceptance.json").read_text(encoding="utf-8"), encoding="utf-8")
    (run_dir / "delivery-summary.md").write_text("树状配置 relevance 测试归档完成。\n", encoding="utf-8")
else:
    raise SystemExit(f"unexpected fake codex stage: {stage}")

print(json.dumps({"type": "thread.started", "thread_id": "codex-" + stage}))
PY
SH
chmod +x "$fakebin/codex"

cat >"$fakebin/pi" <<'SH'
#!/usr/bin/env bash
# 文件功能目的：模拟子代理会话，要求 prompt 先给出 relevance check，并按职责写出 relevant true/false artifact。
set -euo pipefail

model=""
prompt=""
printf 'pi argv: %s\n' "$*" >>"${AGENT_CALL_LOG:?}"
while (($#)); do
  case "$1" in
    --model)
      model="$2"
      shift 2
      ;;
    --session|--mode|--thinking)
      shift 2
      ;;
    *)
      prompt="$1"
      shift
      ;;
  esac
done
if [[ -z "$prompt" ]]; then
  prompt="$(cat || true)"
fi

python3 - "$prompt" "$model" <<'PY'
import json
import os
import pathlib
import re
import sys

prompt, model = sys.argv[1:3]
if "relevant" not in prompt or "无关" not in prompt:
    raise SystemExit("subagent prompt must require relevance check before exploration")

name_match = re.search(r"^SUBAGENT_NAME=(.+)$", prompt, re.M)
purpose_match = re.search(r"^SUBAGENT_PURPOSE=(.+)$", prompt, re.M)
change_match = re.search(r"^CURRENT_CHANGE=(.+)$", prompt, re.M)
if not name_match:
    raise SystemExit("subagent prompt must expose SUBAGENT_NAME")
name = name_match.group(1).strip()
purpose = purpose_match.group(1).strip() if purpose_match else "阶段前置子代理"
change = change_match.group(1).strip() if change_match else "1-纯CLI配置变更"

with pathlib.Path(os.environ["AGENT_CALL_LOG"]).open("a", encoding="utf-8") as handle:
    handle.write(f"pi-call name={name} model={model or '<default>'}\n")

if name == "浏览器路径测试员":
    body = {
        "name": name,
        "change_name": change,
        "purpose": purpose,
        "status": "skipped",
        "relevant": False,
        "irrelevant_reason": "当前提案只修改 CLI 配置解析，不涉及 Web UI、浏览器路由或页面交互。",
        "summary": "无关，未启动浏览器探索。",
        "evidence": ["docs/changes/1-纯CLI配置变更/spec.md"],
        "findings": []
    }
else:
    body = {
        "name": name,
        "change_name": change,
        "purpose": purpose,
        "status": "success",
        "relevant": True,
        "summary": name + " 已完成职责范围内检查。",
        "evidence": ["test-results/codex-workflow-cli/tree-config/subagent-relevance.log"],
        "findings": []
    }

print(json.dumps({"type": "session", "id": "pi-" + name}))
print(json.dumps({
    "type": "message",
    "message": {
        "role": "assistant",
        "content": [{"type": "text", "text": json.dumps(body, ensure_ascii=False)}]
    }
}, ensure_ascii=False))
PY
SH
chmod +x "$fakebin/pi"
cp "$fakebin/pi" "$fakebin/agy"

project="$tmp/project"
change="1-纯CLI配置变更"
mkdir -p "$project/docs/changes/$change/tests"
git -C "$project" init -q
git -C "$project" config user.email "test@example.com"
git -C "$project" config user.name "Test User"

cat >"$project/docs/changes/$change/brief.md" <<'MD'
# 纯 CLI 配置变更

这个临时 change 用于验证纯 CLI 配置变更下，浏览器路径测试员应判断无关并快速返回。
MD

cat >"$project/docs/changes/$change/proposal.md" <<'MD'
# 纯 CLI 配置变更

## 背景

本变更只涉及 CLI 配置解析，不涉及 Web UI。
MD

cat >"$project/docs/changes/$change/design.md" <<'MD'
# 设计

通过 task 勾选表达 execution 完成，通过 fake QA 表达 CLI 路径验证完成。
MD

cat >"$project/docs/changes/$change/spec.md" <<'MD'
# 规格

### 需求：纯 CLI 配置变更

系统必须完成 CLI 配置解析变更，不触发浏览器路径探索。

#### 场景：CLI 配置更新

- **当** 用户运行 oz flow run
- **则** execution 完成 task
- **且** 浏览器路径测试员标记无关
MD

cat >"$project/docs/changes/$change/task.md" <<'MD'
# 任务

- [ ] 1.1 完成 CLI 配置解析变更
MD

cat >"$project/docs/changes/$change/tests/test_contract.sh" <<'SH'
#!/usr/bin/env bash
# 文件功能目的：验证 execution 真实修改了当前 change 的 task 状态。
set -euo pipefail
grep -qF "[x]" docs/changes/1-纯CLI配置变更/task.md
SH
chmod +x "$project/docs/changes/$change/tests/test_contract.sh"

cat >"$project/docs/changes/$change/acceptance.json" <<'JSON'
{
  "summary": "纯 CLI 配置变更的最小验收合同",
  "required_tests": [
    {
      "id": "cli-config-contract",
      "source": "change_contract",
      "path": "docs/changes/1-纯CLI配置变更/tests/test_contract.sh",
      "command": "bash docs/changes/1-纯CLI配置变更/tests/test_contract.sh",
      "purpose": "证明 execution 完成 CLI 配置变更任务",
      "assertions": ["task.md 中 CLI 配置变更任务从未完成变为已完成"]
    }
  ],
  "required_evidence": []
}
JSON

cat >"$project/oz-flow.yaml" <<'YAML'
parallel: true
max_review_iterations: 1
stages:
  execution:
    agent: codex
    model: codex-exec-model
    reasoning: low
    before:
      - name: 代码库侦察员
        purpose: 搜索相关源码、测试、配置和既有实现约定
        agent: pi
        subagent: explore
        required: false
      - name: 外部资料研究员
        purpose: 查询外部库文档和开源实现
        agent: pi
        subagent: librarian
        required: false
  review:
    agent: codex
    reasoning: high
    before:
      - name: 目标核对审核员
        purpose: 核对实现是否满足 proposal/spec/task/acceptance
        agent: pi
        required: true
      - name: 测试有效性审核员
        purpose: 判断测试是否覆盖真实业务路径和失败场景
        agent: pi
        required: true
      - name: 安全风险审核员
        purpose: 检查权限、命令、远程、凭据、输入边界风险
        agent: pi
        required: true
      - name: 上下文一致性审核员
        purpose: 检查是否违背现有架构和配置约定
        agent: pi
        required: true
  qa:
    agent: codex
    reasoning: high
    before:
      - name: CLI/API 测试员
        purpose: 执行命令行或接口真实路径
        agent: pi
        required: true
      - name: 浏览器路径测试员
        purpose: 执行页面真实用户路径
        agent: pi
        model: pi-browser-model
        required: true
      - name: 回归场景测试员
        purpose: 覆盖邻近功能回归
        agent: pi
        required: true
  fix:
    agent: codex
    reasoning: low
  archive:
    agent: codex
    reasoning: low
validation:
  limit: 2
  commands: []
YAML

git -C "$project" add .
git -C "$project" commit -qm init

note "运行 oz flow run，验证 stages.before 子代理默认启动与 relevant:false"
AGENT_CALL_LOG="$agent_log" \
WO_TEST_REPO="$project" \
XDG_STATE_HOME="$tmp/state" \
HOME="$tmp/home" \
PATH="$fakebin:/usr/bin:/bin" \
  bash -c 'cd "$1" && "$2" flow run --change "$3" --json' _ "$project" "$wo_bin" "$change" >"$tmp/run.jsonl" 2>"$tmp/run.err" || {
    cat "$tmp/run.err" | tee -a "$log"
    fail "oz flow run 未能完成树状配置 relevance 场景"
  }

cat "$tmp/run.jsonl" >>"$log"
cat "$agent_log" >>"$log"

state_path="$(XDG_STATE_HOME="$tmp/state" python3 - <<'PY'
import os
import pathlib

states = sorted(pathlib.Path(os.environ["XDG_STATE_HOME"]).glob("oz/flow/repos/*/runs/*/state.json"))
if not states:
    raise SystemExit("no state.json found")
print(states[-1])
PY
)"
cp "$state_path" "$state_evidence"

python3 - "$state_path" "$agent_log" <<'PY'
import json
import pathlib
import sys

state_path = pathlib.Path(sys.argv[1])
agent_log = pathlib.Path(sys.argv[2])
state = json.loads(state_path.read_text(encoding="utf-8"))
log = agent_log.read_text(encoding="utf-8")

if state.get("status") != "done" or state.get("stage") != "done":
    raise SystemExit(f"workflow should finish done, got status={state.get('status')} stage={state.get('stage')}")

if "-m codex-exec-model" not in log:
    raise SystemExit("execution main stage did not receive -m codex-exec-model")
if "pi-call name=浏览器路径测试员 model=pi-browser-model" not in log:
    raise SystemExit("browser subagent did not receive --model pi-browser-model")
if "pi-call name=CLI/API 测试员 model=<default>" not in log:
    raise SystemExit("unconfigured CLI/API subagent should use CLI default model without --model")

expected = [
    "代码库侦察员",
    "外部资料研究员",
    "目标核对审核员",
    "测试有效性审核员",
    "安全风险审核员",
    "上下文一致性审核员",
    "CLI/API 测试员",
    "浏览器路径测试员",
    "回归场景测试员",
]
for name in expected:
    if f"pi-call name={name} " not in log:
        raise SystemExit(f"missing subagent call: {name}")

browser = None
for path in state_path.parent.rglob("*.json"):
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except Exception:
        continue
    stack = [data]
    while stack:
        item = stack.pop()
        if isinstance(item, dict):
            if item.get("name") == "浏览器路径测试员":
                browser = item
                break
            stack.extend(item.values())
        elif isinstance(item, list):
            stack.extend(item)
    if browser:
        break

if not browser:
    raise SystemExit("missing browser subagent artifact")
if browser.get("relevant") is not False:
    raise SystemExit(f"browser subagent should write relevant=false, got {browser}")
if not browser.get("irrelevant_reason"):
    raise SystemExit("browser subagent relevant=false artifact must include irrelevant_reason")
if browser.get("findings"):
    raise SystemExit(f"irrelevant browser subagent must not produce findings: {browser}")
PY

note "已生成 evidence: $state_evidence"
note "PASS: subagent-relevance-contract"
