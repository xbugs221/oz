#!/usr/bin/env bash
# 文件功能目的：验证 run-acceptance 能在真实临时项目中执行 active change 的 required_tests 并检查 evidence。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/37-执行验收合同测试并汇总结果/success"
LOG="$RESULT_DIR/contract.log"
TMP="$(mktemp -d)"

cleanup() {
  rm -rf "$TMP"
}

fail() {
  printf 'FAIL: %s\n' "$*" | tee -a "$LOG" >&2
  exit 1
}

note() {
  printf '%s\n' "$*" | tee -a "$LOG"
}

write_change_file() {
  local path="$1"
  local body="$2"
  mkdir -p "$(dirname "$path")"
  printf '%s\n' "$body" >"$path"
}

trap cleanup EXIT
rm -rf "$RESULT_DIR"
mkdir -p "$RESULT_DIR"

note "build real oz binary"
OZ_BIN="$TMP/oz"
(cd "$ROOT" && go build -o "$OZ_BIN" ./cmd/oz) >>"$LOG" 2>&1

PROJECT="$TMP/project"
CHANGE="1-验收执行样例"
mkdir -p "$PROJECT/docs/changes/$CHANGE/tests"
(
  cd "$PROJECT"
  git init -q
  git config user.email "test@example.com"
  git config user.name "Test User"
  printf 'demo project for acceptance run success\n' >README.md
)

write_change_file "$PROJECT/docs/changes/$CHANGE/brief.md" "# 简报
本变更用于验证 run-acceptance 成功路径。"
write_change_file "$PROJECT/docs/changes/$CHANGE/proposal.md" "# 提案
执行一个会写入 evidence 的 required test。"
write_change_file "$PROJECT/docs/changes/$CHANGE/design.md" "# 设计
测试脚本读取 README 并写入 runtime log。"
write_change_file "$PROJECT/docs/changes/$CHANGE/spec.md" "### 需求：验收执行样例

系统必须执行 required test 并检查 runtime evidence。

#### 场景：成功写入 evidence

- 测试文件：docs/changes/$CHANGE/tests/pass_contract.sh
- 真实数据来源：README.md
- 入口路径：oz flow run-acceptance --change $CHANGE --json
- 关键断言：runtime log 存在。
- 剩余风险：只覆盖单测试成功路径。"
write_change_file "$PROJECT/docs/changes/$CHANGE/task.md" "- [ ] 运行 required test
- [ ] 检查 evidence"

cat >"$PROJECT/docs/changes/$CHANGE/tests/pass_contract.sh" <<'SH'
#!/usr/bin/env bash
# 文件功能目的：作为临时项目中的真实 required test，读取业务输入并写入 runtime evidence。
set -euo pipefail
mkdir -p test-results/acceptance-run-demo
grep -q 'demo project for acceptance run success' README.md
printf 'acceptance-run-success-log: README business input verified\n' > test-results/acceptance-run-demo/runtime.log
SH
chmod +x "$PROJECT/docs/changes/$CHANGE/tests/pass_contract.sh"

cat >"$PROJECT/docs/changes/$CHANGE/acceptance.json" <<JSON
{
  "summary": "成功执行 required test 并写入 runtime evidence",
  "coverage": [
    {
      "spec": "需求：验收执行样例 / 场景：成功写入 evidence",
      "tests": ["success-required-test"],
      "evidence": ["success-runtime-log"],
      "risk": "只覆盖单测试成功路径"
    }
  ],
  "required_tests": [
    {
      "id": "success-required-test",
      "source": "change_contract",
      "path": "docs/changes/$CHANGE/tests/pass_contract.sh",
      "command": "bash docs/changes/$CHANGE/tests/pass_contract.sh",
      "purpose": "读取 README 并写入 test-results/acceptance-run-demo/runtime.log",
      "assertions": ["required test 写入 test-results/acceptance-run-demo/runtime.log 并证明业务输入被读取"]
    }
  ],
  "required_evidence": [
    {
      "id": "success-runtime-log",
      "kind": "runtime_log",
      "path": "test-results/acceptance-run-demo/runtime.log",
      "purpose": "证明 required test 真实运行并读取了 README 业务输入"
    }
  ]
}
JSON

(
  cd "$PROJECT"
  git add .
  git commit -qm init
)

note "run acceptance contract in temporary project"
set +e
(
  cd "$PROJECT"
  "$OZ_BIN" flow run-acceptance --change "$CHANGE" --json
) >"$RESULT_DIR/run.json" 2>"$RESULT_DIR/run.err"
code=$?
set -e
[[ "$code" -eq 0 ]] || fail "run-acceptance should pass, exit=$code, stderr=$(cat "$RESULT_DIR/run.err")"

python3 - "$RESULT_DIR/run.json" "$PROJECT" "$CHANGE" <<'PY' >>"$LOG" 2>&1
import json
import pathlib
import sys

result_path = pathlib.Path(sys.argv[1])
project = pathlib.Path(sys.argv[2])
change = sys.argv[3]
payload = json.loads(result_path.read_text(encoding="utf-8").strip().splitlines()[-1])
if payload.get("change") != change:
    raise SystemExit(f"change mismatch: {payload.get('change')!r}")
if payload.get("valid") is not True or payload.get("status") != "passed":
    raise SystemExit(f"expected passed result, got {payload!r}")
summary = payload.get("summary") or {}
if summary.get("total") != 1 or summary.get("passed") != 1 or summary.get("failed") != 0:
    raise SystemExit(f"bad summary: {summary!r}")
tests = payload.get("tests") or []
if len(tests) != 1 or tests[0].get("id") != "success-required-test" or tests[0].get("status") != "passed":
    raise SystemExit(f"bad tests: {tests!r}")
log_path = project / tests[0]["log_path"]
if not log_path.is_file():
    raise SystemExit(f"missing per-test log: {log_path}")
evidence = payload.get("evidence") or []
if len(evidence) != 1 or evidence[0].get("id") != "success-runtime-log" or evidence[0].get("status") != "present":
    raise SystemExit(f"bad evidence: {evidence!r}")
if not (project / "test-results/acceptance-run-demo/runtime.log").is_file():
    raise SystemExit("runtime evidence file missing")
print("success acceptance run JSON and runtime evidence verified")
PY

note "success contract passed; evidence: test-results/37-执行验收合同测试并汇总结果/success/contract.log"

