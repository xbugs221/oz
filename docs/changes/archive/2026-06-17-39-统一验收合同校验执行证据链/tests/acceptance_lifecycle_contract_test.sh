#!/usr/bin/env bash
# 文件功能目的：验证 acceptance 校验、执行和 QA 证据链通过共享 lifecycle 诊断边界保持一致。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/39-acceptance-lifecycle"
LOG="$RESULT_DIR/contract.log"
TMP_DIR="$RESULT_DIR/tmp"
OZ_BIN="$RESULT_DIR/oz"

rm -rf "$TMP_DIR"
mkdir -p "$RESULT_DIR" "$TMP_DIR"
: >"$LOG"

note() {
  printf '[acceptance-lifecycle] %s\n' "$*" | tee -a "$LOG"
}

fail() {
  printf '[acceptance-lifecycle] FAIL: %s\n' "$*" | tee -a "$LOG" >&2
  exit 1
}

assert_rg() {
  local pattern="$1"
  local path="$2"
  local message="$3"
  if ! rg -n "$pattern" "$path" >>"$LOG" 2>&1; then
    fail "$message"
  fi
}

note "检查 lifecycle 共享诊断边界"
assert_rg 'LifecycleDiagnostic|LifecycleResult|ValidateLifecycle|BuildLifecycle' "$ROOT/internal/acceptance" "缺少 acceptance lifecycle 诊断边界"
assert_rg 'LifecycleDiagnostic|ValidateLifecycle|BuildLifecycle' "$ROOT/internal/ozcli/cmd_validate.go" "oz validate 未复用 acceptance lifecycle"
assert_rg 'LifecycleDiagnostic|ValidateLifecycle|BuildLifecycle' "$ROOT/internal/app/acceptance_preflight.go" "acceptance preflight 未复用 lifecycle"
assert_rg 'Diagnostics|LifecycleDiagnostic|ValidateLifecycle|BuildLifecycle' "$ROOT/internal/app/acceptance_run.go" "run-acceptance 未暴露 lifecycle diagnostics"

note "构建真实 oz 二进制"
(cd "$ROOT" && go build -o "$OZ_BIN" ./cmd/oz) >>"$LOG" 2>&1 || fail "go build ./cmd/oz 失败"

PROJECT="$TMP_DIR/project"
CHANGE="1-验收生命周期样例"
mkdir -p "$PROJECT/docs/changes/$CHANGE/tests"
cd "$PROJECT"
git init >>"$LOG" 2>&1
git config user.email "oz-test@example.com"
git config user.name "oz test"

cat >"docs/changes/$CHANGE/brief.md" <<'EOF'
# 简报：验收生命周期样例

用于验证 acceptance lifecycle 的最小真实提案。
EOF
cat >"docs/changes/$CHANGE/proposal.md" <<'EOF'
# 提案：验收生命周期样例

本样例通过 required_tests 写入 runtime evidence。
EOF
cat >"docs/changes/$CHANGE/design.md" <<'EOF'
# 设计：验收生命周期样例

测试脚本写入 test-results/lifecycle/runtime.log。
EOF
cat >"docs/changes/$CHANGE/spec.md" <<'EOF'
### 需求：验收生命周期样例

系统必须运行 required test 并生成 runtime evidence。

#### 场景：required test 生成 runtime evidence

- 测试：docs/changes/1-验收生命周期样例/tests/lifecycle_contract_test.sh
- 真实数据来源：测试脚本写入的 runtime log。
- 入口路径：oz validate 和 oz flow run-acceptance。
- 关键断言：runtime evidence 存在且 run-acceptance JSON 可追溯。
EOF
cat >"docs/changes/$CHANGE/task.md" <<'EOF'
- [ ] 编写 runtime evidence 测试
- [ ] 运行 acceptance lifecycle
EOF
cat >"docs/changes/$CHANGE/tests/lifecycle_contract_test.sh" <<'EOF'
#!/usr/bin/env bash
# 文件功能目的：为 acceptance lifecycle 样例生成真实 runtime evidence。
set -euo pipefail
mkdir -p test-results/lifecycle
printf 'acceptance lifecycle runtime evidence\n' > test-results/lifecycle/runtime.log
grep -q 'acceptance lifecycle' test-results/lifecycle/runtime.log
EOF
chmod +x "docs/changes/$CHANGE/tests/lifecycle_contract_test.sh"
cat >"docs/changes/$CHANGE/acceptance.json" <<'EOF'
{
  "summary": "acceptance lifecycle fixture",
  "coverage": [
    {
      "spec": "需求：验收生命周期样例 / 场景：required test 生成 runtime evidence",
      "tests": ["lifecycle-contract"],
      "evidence": ["lifecycle-runtime-log"],
      "risk": "fixture only covers one runtime evidence producer"
    }
  ],
  "required_tests": [
    {
      "id": "lifecycle-contract",
      "source": "change_contract",
      "path": "docs/changes/1-验收生命周期样例/tests/lifecycle_contract_test.sh",
      "command": "bash docs/changes/1-验收生命周期样例/tests/lifecycle_contract_test.sh",
      "purpose": "produce lifecycle-runtime-log at test-results/lifecycle/runtime.log",
      "assertions": [
        "required test writes lifecycle-runtime-log to test-results/lifecycle/runtime.log and verifies the business marker"
      ]
    }
  ],
  "required_evidence": [
    {
      "id": "lifecycle-runtime-log",
      "kind": "runtime_log",
      "path": "test-results/lifecycle/runtime.log",
      "purpose": "prove the required test generated runtime evidence"
    }
  ]
}
EOF

git add docs >>"$LOG" 2>&1
git commit -m init >>"$LOG" 2>&1

note "运行 oz validate 和 run-acceptance"
"$OZ_BIN" validate "$CHANGE" --json >"$RESULT_DIR/validate.json" 2>>"$LOG" || fail "oz validate 应通过旧 schema 合同"
"$OZ_BIN" flow run-acceptance --change "$CHANGE" --json >"$RESULT_DIR/run-acceptance.json" 2>>"$LOG" || fail "run-acceptance 应执行 required test 并通过"

if ! rg -n '"valid":true|"status":"passed"' "$RESULT_DIR/run-acceptance.json" >>"$LOG" 2>&1; then
  fail "run-acceptance JSON 未保留通过状态"
fi
python3 - "$RESULT_DIR/run-acceptance.json" >>"$LOG" 2>&1 <<'PY' || fail "run-acceptance JSON 缺少 lifecycle producer/coverage 证据链绑定"
import json
import sys

with open(sys.argv[1], encoding="utf-8") as fh:
    payload = json.load(fh)

coverage = payload.get("coverage") or []
producers = payload.get("producers") or []

has_coverage = any(
    "lifecycle-contract" in item.get("tests", [])
    and "lifecycle-runtime-log" in item.get("evidence", [])
    for item in coverage
)
has_producer = any(
    item.get("evidence_id") == "lifecycle-runtime-log"
    and "lifecycle-contract" in item.get("tests", [])
    and item.get("verified") is True
    for item in producers
)

if not has_coverage or not has_producer:
    raise SystemExit(f"missing trace: coverage={coverage!r} producers={producers!r}")
PY

note "PASS: acceptance lifecycle contract"
