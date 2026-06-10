#!/usr/bin/env bash
# 文件功能目的：验证 sealed run 启动前同时检查 codex/pi，并拒绝第三后端配置。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
LOG="$ROOT/test-results/14-cli-preflight.log"
STATE_SNAPSHOT="$ROOT/test-results/14-state-clean.txt"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$(dirname "$LOG")"
: >"$LOG"
: >"$STATE_SNAPSHOT"

# note 记录真实 CLI 行为，作为 acceptance evidence。
note() {
  printf '[cli-preflight] %s\n' "$*" | tee -a "$LOG"
}

# fail 报告业务契约失败。
fail() {
  printf '[cli-preflight] FAIL: %s\n' "$*" | tee -a "$LOG" >&2
  exit 1
}

# write_demo_change 创建一个最小真实 oz change，供真实 CLI 启动 sealed run。
write_demo_change() {
  local repo="$1"
  local change="$2"
  mkdir -p "$repo/docs/changes/$change/tests"
  printf '# brief\n' >"$repo/docs/changes/$change/brief.md"
  printf '# proposal\n' >"$repo/docs/changes/$change/proposal.md"
  printf '# design\n' >"$repo/docs/changes/$change/design.md"
  cat >"$repo/docs/changes/$change/spec.md" <<'MD'
# spec

### 需求：演示

系统必须提供演示合同。

#### 场景：演示路径

- **当** 执行测试
- **则** brief 存在
MD
  printf -- '- [ ] demo task\n' >"$repo/docs/changes/$change/task.md"
  cat >"$repo/docs/changes/$change/tests/test_smoke.sh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
test -f docs/changes/1-演示检查/brief.md
SH
  chmod +x "$repo/docs/changes/$change/tests/test_smoke.sh"
  cat >"$repo/docs/changes/$change/acceptance.json" <<'JSON'
{
  "summary": "demo",
  "coverage": [
    {
      "spec": "需求：demo / 场景：demo",
      "tests": ["demo-smoke"],
      "evidence": [],
      "risk": "demo only"
    }
  ],
  "required_tests": [
    {
      "id": "demo-smoke",
      "source": "change_contract",
      "path": "docs/changes/1-演示检查/tests/test_smoke.sh",
      "command": "bash docs/changes/1-演示检查/tests/test_smoke.sh",
      "purpose": "demo",
      "assertions": ["brief exists"]
    }
  ],
  "required_evidence": []
}
JSON
}

note "编译真实 wo CLI"
WO="$TMP/wo"
(cd "$ROOT" && go build -o "$WO" ./cmd/wo) 2>&1 | tee -a "$LOG"
OZ="$TMP/oz"
(cd "$ROOT" && go build -o "$OZ" ./cmd/oz) 2>&1 | tee -a "$LOG"

note "创建只有 fake codex、没有 pi 的 PATH"
FAKEBIN="$TMP/bin"
mkdir -p "$FAKEBIN"
cat >"$FAKEBIN/codex" <<'SH'
#!/usr/bin/env bash
printf '{"type":"thread.started","thread_id":"fake-thread"}\n'
exit 0
SH
chmod +x "$FAKEBIN/codex"
ln -s "$(command -v git)" "$FAKEBIN/git"
ln -s "$OZ" "$FAKEBIN/oz"

PROJECT="$TMP/project"
CHANGE="1-演示检查"
mkdir -p "$PROJECT"
(cd "$PROJECT" && git init -q && git config user.email test@example.com && git config user.name Test)
write_demo_change "$PROJECT" "$CHANGE"
(cd "$PROJECT" && git add . && git commit -q -m init)

note "缺少 pi 时必须在创建运行态前失败，并提示安装"
RUN_HOME="$TMP/home-missing-pi"
RUN_STATE="$TMP/state-missing-pi"
mkdir -p "$RUN_HOME" "$RUN_STATE"
set +e
(
  cd "$PROJECT"
  HOME="$RUN_HOME" XDG_STATE_HOME="$RUN_STATE" PATH="$FAKEBIN" "$WO" run --change "$CHANGE" --json
) >"$TMP/missing-pi.out" 2>"$TMP/missing-pi.err"
code=$?
set -e
cat "$TMP/missing-pi.out" "$TMP/missing-pi.err" | tee -a "$LOG"
[[ "$code" -ne 0 ]] || fail "缺少 pi 时 run 不应成功"
grep -Eiq 'pi' "$TMP/missing-pi.out" "$TMP/missing-pi.err" || fail "错误输出必须指出缺少 pi"
grep -Eiq '安装|install' "$TMP/missing-pi.out" "$TMP/missing-pi.err" || fail "错误输出必须提示用户先安装 CLI"
if [[ -d "$RUN_STATE/oz/runs" ]]; then
  find "$RUN_STATE/oz/runs" -maxdepth 2 -type f | sort >"$STATE_SNAPSHOT"
  [[ ! -s "$STATE_SNAPSHOT" ]] || fail "启动前检查失败后不应创建 run state"
fi
printf 'no run state created\n' >"$STATE_SNAPSHOT"

note "第三后端配置必须按未知工具失败"
legacy_tool="open""code"
cat >"$PROJECT/wo.yaml" <<YAML
wo:
  workflow:
    defaults:
      tool: ${legacy_tool}
YAML
set +e
(
  cd "$PROJECT"
  HOME="$TMP/home-invalid-tool" XDG_STATE_HOME="$TMP/state-invalid-tool" PATH="$FAKEBIN" "$WO" run --change "$CHANGE" --json
) >"$TMP/invalid-tool.out" 2>"$TMP/invalid-tool.err"
code=$?
set -e
cat "$TMP/invalid-tool.out" "$TMP/invalid-tool.err" | tee -a "$LOG"
[[ "$code" -ne 0 ]] || fail "第三后端配置不应启动成功"
grep -Eiq '未知|unknown|agent tool|invalid' "$TMP/invalid-tool.out" "$TMP/invalid-tool.err" || fail "第三后端配置必须报告未知工具"

note "contract passed: startup preflight and allowlist are strict"
