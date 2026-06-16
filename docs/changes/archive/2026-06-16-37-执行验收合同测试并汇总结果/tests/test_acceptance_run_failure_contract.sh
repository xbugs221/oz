#!/usr/bin/env bash
# 文件功能目的：验证 run-acceptance 在 required test 失败时仍执行后续测试并输出完整失败汇总。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/37-执行验收合同测试并汇总结果/failure"
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
CHANGE="1-验收失败样例"
mkdir -p "$PROJECT/docs/changes/$CHANGE/tests"
(
  cd "$PROJECT"
  git init -q
  git config user.email "test@example.com"
  git config user.name "Test User"
  printf 'demo project for acceptance run failure\n' >README.md
)

write_change_file "$PROJECT/docs/changes/$CHANGE/brief.md" "# 简报
本变更用于验证 run-acceptance 失败汇总路径。"
write_change_file "$PROJECT/docs/changes/$CHANGE/proposal.md" "# 提案
同时运行一个通过测试和一个失败测试。"
write_change_file "$PROJECT/docs/changes/$CHANGE/design.md" "# 设计
失败测试先写入日志再返回非零，证明执行器可以汇总失败。"
write_change_file "$PROJECT/docs/changes/$CHANGE/spec.md" "### 需求：验收失败样例

系统必须在一个 required test 失败时继续执行后续 required test。

#### 场景：失败不短路

- 测试文件：docs/changes/$CHANGE/tests/fail_contract.sh
- 真实数据来源：README.md
- 入口路径：oz flow run-acceptance --change $CHANGE --json
- 关键断言：两个测试都执行，JSON 标记一个 passed 一个 failed。
- 剩余风险：不覆盖并行执行。"
write_change_file "$PROJECT/docs/changes/$CHANGE/task.md" "- [ ] 运行两个 required tests
- [ ] 检查失败汇总"

cat >"$PROJECT/docs/changes/$CHANGE/tests/fail_contract.sh" <<'SH'
#!/usr/bin/env bash
# 文件功能目的：作为临时项目中的失败 required test，写入 failure evidence 后返回非零。
set -euo pipefail
mkdir -p test-results/acceptance-run-failure
printf 'acceptance-run-failure-log: failing required test executed\n' > test-results/acceptance-run-failure/fail.log
exit 7
SH
chmod +x "$PROJECT/docs/changes/$CHANGE/tests/fail_contract.sh"

cat >"$PROJECT/docs/changes/$CHANGE/tests/pass_after_failure.sh" <<'SH'
#!/usr/bin/env bash
# 文件功能目的：作为失败后的后续 required test，证明执行器不会在第一个失败处短路。
set -euo pipefail
mkdir -p test-results/acceptance-run-failure
grep -q 'demo project for acceptance run failure' README.md
printf 'acceptance-run-failure-log: pass after failure executed\n' > test-results/acceptance-run-failure/pass.log
SH
chmod +x "$PROJECT/docs/changes/$CHANGE/tests/pass_after_failure.sh"

cat >"$PROJECT/docs/changes/$CHANGE/acceptance.json" <<JSON
{
  "summary": "失败时仍执行并汇总全部 required tests",
  "coverage": [
    {
      "spec": "需求：验收失败样例 / 场景：失败不短路",
      "tests": ["failing-required-test", "pass-after-failure-test"],
      "evidence": ["failure-runtime-log", "pass-after-failure-log"],
      "risk": "不覆盖并行执行"
    }
  ],
  "required_tests": [
    {
      "id": "failing-required-test",
      "source": "change_contract",
      "path": "docs/changes/$CHANGE/tests/fail_contract.sh",
      "command": "bash docs/changes/$CHANGE/tests/fail_contract.sh",
      "purpose": "写入 test-results/acceptance-run-failure/fail.log 后返回非零",
      "assertions": ["失败 required test 写入 test-results/acceptance-run-failure/fail.log 且 exit code 被 JSON 记录"]
    },
    {
      "id": "pass-after-failure-test",
      "source": "change_contract",
      "path": "docs/changes/$CHANGE/tests/pass_after_failure.sh",
      "command": "bash docs/changes/$CHANGE/tests/pass_after_failure.sh",
      "purpose": "第一个测试失败后仍写入 test-results/acceptance-run-failure/pass.log",
      "assertions": ["失败不短路，后续 required test 写入 test-results/acceptance-run-failure/pass.log 并出现在 JSON tests 列表"]
    }
  ],
  "required_evidence": [
    {
      "id": "failure-runtime-log",
      "kind": "runtime_log",
      "path": "test-results/acceptance-run-failure/fail.log",
      "purpose": "证明失败 required test 已实际运行"
    },
    {
      "id": "pass-after-failure-log",
      "kind": "runtime_log",
      "path": "test-results/acceptance-run-failure/pass.log",
      "purpose": "证明失败后续 required test 仍实际运行"
    }
  ]
}
JSON

(
  cd "$PROJECT"
  git add .
  git commit -qm init
)

note "run acceptance contract and expect nonzero result with complete JSON"
set +e
(
  cd "$PROJECT"
  "$OZ_BIN" flow run-acceptance --change "$CHANGE" --json
) >"$RESULT_DIR/run.json" 2>"$RESULT_DIR/run.err"
code=$?
set -e
[[ "$code" -ne 0 ]] || fail "run-acceptance should fail because one required test exits nonzero"
[[ -s "$RESULT_DIR/run.json" ]] || fail "run-acceptance must emit JSON even on failure; stderr=$(cat "$RESULT_DIR/run.err")"

python3 - "$RESULT_DIR/run.json" "$PROJECT" "$CHANGE" <<'PY' >>"$LOG" 2>&1
import json
import pathlib
import sys

result_path = pathlib.Path(sys.argv[1])
project = pathlib.Path(sys.argv[2])
change = sys.argv[3]
text = result_path.read_text(encoding="utf-8").strip()
if not text:
    raise SystemExit("run-acceptance must emit JSON even on failure")
payload = json.loads(text.splitlines()[-1])
if payload.get("change") != change:
    raise SystemExit(f"change mismatch: {payload.get('change')!r}")
if payload.get("valid") is not False or payload.get("status") != "failed":
    raise SystemExit(f"expected failed result, got {payload!r}")
summary = payload.get("summary") or {}
if summary.get("total") != 2 or summary.get("passed") != 1 or summary.get("failed") != 1:
    raise SystemExit(f"bad failure summary: {summary!r}")
tests = {item.get("id"): item for item in payload.get("tests") or []}
if set(tests) != {"failing-required-test", "pass-after-failure-test"}:
    raise SystemExit(f"missing test results: {tests!r}")
if tests["failing-required-test"].get("status") != "failed" or tests["failing-required-test"].get("exit_code") != 7:
    raise SystemExit(f"bad failing test result: {tests['failing-required-test']!r}")
if tests["pass-after-failure-test"].get("status") != "passed":
    raise SystemExit(f"pass-after-failure test did not run: {tests['pass-after-failure-test']!r}")
for item in tests.values():
    if not (project / item["log_path"]).is_file():
        raise SystemExit(f"missing per-test log: {item['log_path']}")
for rel in ["test-results/acceptance-run-failure/fail.log", "test-results/acceptance-run-failure/pass.log"]:
    if not (project / rel).is_file():
        raise SystemExit(f"missing runtime evidence: {rel}")
print("failure acceptance run JSON, exit code, and all logs verified")
PY

note "failure contract passed; evidence: test-results/37-执行验收合同测试并汇总结果/failure/contract.log"
