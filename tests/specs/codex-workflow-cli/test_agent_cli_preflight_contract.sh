#!/usr/bin/env bash
# 文件功能目的：验证 sealed run 启动前同时检查 codex/pi/agy，并在缺失 CLI 或无效后端配置时不创建运行态。
# Sources: 14-精简后端为-codex-pi-并迁移测试, 15-支持-agy-cli作为pi候选
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
LOG="$ROOT/test-results/spec-agent-cli-preflight.log"
STATE_SNAPSHOT="$ROOT/test-results/spec-agent-cli-preflight-state.txt"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$(dirname "$LOG")"
: >"$LOG"
: >"$STATE_SNAPSHOT"

# note 记录真实 CLI 输出，作为长期规格证据。
note() {
  printf '[agent-cli-preflight] %s\n' "$*" | tee -a "$LOG"
}

# fail 报告启动前检查合同失败。
fail() {
  printf '[agent-cli-preflight] FAIL: %s\n' "$*" | tee -a "$LOG" >&2
  exit 1
}

# assert_no_run_state 确认失败发生在 sealed run 状态创建之前。
assert_no_run_state() {
  local state_root="$1"
  local output_file="$2"
  if [[ -d "$state_root/oz/flow/repos" ]]; then
    find "$state_root/oz/flow/repos" -path '*/runs/*/state.json' -type f | sort >"$output_file"
    [[ ! -s "$output_file" ]] || fail "启动前检查失败后不应创建 run state"
  else
    : >"$output_file"
  fi
}

# write_demo_change 创建最小 active change，让测试走真实 oz flow run 入口。
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

note "编译真实 wo/oz CLI"
WO="$TMP/wo"
OZ="$TMP/oz"
(cd "$ROOT" && go build -o "$WO" ./cmd/oz) 2>&1 | tee -a "$LOG"
(cd "$ROOT" && go build -o "$OZ" ./cmd/oz) 2>&1 | tee -a "$LOG"

PROJECT="$TMP/project"
CHANGE="1-演示检查"
mkdir -p "$PROJECT"
(cd "$PROJECT" && git init -q && git config user.email test@example.com && git config user.name Test)
write_demo_change "$PROJECT" "$CHANGE"
(cd "$PROJECT" && git add . && git commit -q -m init)

note "缺少 pi 时必须失败且不创建 run state"
FAKEBIN="$TMP/bin-missing-pi"
mkdir -p "$FAKEBIN"
cat >"$FAKEBIN/codex" <<'SH'
#!/usr/bin/env bash
printf '{"type":"thread.started","thread_id":"fake-thread"}\n'
SH
chmod +x "$FAKEBIN/codex"
ln -s "$(command -v git)" "$FAKEBIN/git"
ln -s "$OZ" "$FAKEBIN/oz"
set +e
(
  cd "$PROJECT"
  HOME="$TMP/home-missing-pi" XDG_STATE_HOME="$TMP/state-missing-pi" PATH="$FAKEBIN" "$WO" run --change "$CHANGE" --json
) >"$TMP/missing-pi.out" 2>"$TMP/missing-pi.err"
code=$?
set -e
cat "$TMP/missing-pi.out" "$TMP/missing-pi.err" | tee -a "$LOG"
[[ "$code" -ne 0 ]] || fail "缺少 pi 时 run 不应成功"
grep -Eiq 'pi' "$TMP/missing-pi.out" "$TMP/missing-pi.err" || fail "错误输出必须指出缺少 pi"
grep -Eiq '安装|install' "$TMP/missing-pi.out" "$TMP/missing-pi.err" || fail "错误输出必须提示安装 CLI"
assert_no_run_state "$TMP/state-missing-pi" "$STATE_SNAPSHOT"
printf 'missing pi: no run state created\n' >"$STATE_SNAPSHOT"

note "缺少 agy 时必须失败且不创建 run state"
FAKEBIN="$TMP/bin-missing-agy"
mkdir -p "$FAKEBIN"
cat >"$FAKEBIN/codex" <<'SH'
#!/usr/bin/env bash
printf '{"type":"thread.started","thread_id":"fake-thread"}\n'
SH
cat >"$FAKEBIN/pi" <<'SH'
#!/usr/bin/env bash
printf '{"type":"session","id":"fake-pi"}\n'
SH
chmod +x "$FAKEBIN/codex" "$FAKEBIN/pi"
ln -s "$(command -v git)" "$FAKEBIN/git"
ln -s "$OZ" "$FAKEBIN/oz"
set +e
(
  cd "$PROJECT"
  HOME="$TMP/home-missing-agy" XDG_STATE_HOME="$TMP/state-missing-agy" PATH="$FAKEBIN" "$WO" run --change "$CHANGE" --json
) >"$TMP/missing-agy.out" 2>"$TMP/missing-agy.err"
code=$?
set -e
cat "$TMP/missing-agy.out" "$TMP/missing-agy.err" | tee -a "$LOG"
[[ "$code" -ne 0 ]] || fail "缺少 agy 时 run 不应成功"
grep -Eiq 'agy' "$TMP/missing-agy.out" "$TMP/missing-agy.err" || fail "错误输出必须指出缺少 agy"
grep -Eiq '安装|install' "$TMP/missing-agy.out" "$TMP/missing-agy.err" || fail "错误输出必须提示安装 CLI"
assert_no_run_state "$TMP/state-missing-agy" "$TMP/missing-agy-state.txt"
printf 'missing agy: no run state created\n' >>"$STATE_SNAPSHOT"

note "缺少 codex 时必须失败且不创建 run state"
FAKEBIN="$TMP/bin-missing-codex"
mkdir -p "$FAKEBIN"
cat >"$FAKEBIN/pi" <<'SH'
#!/usr/bin/env bash
printf '{"type":"session","id":"fake-pi"}\n'
SH
cat >"$FAKEBIN/agy" <<'SH'
#!/usr/bin/env bash
printf 'fake agy\n'
SH
chmod +x "$FAKEBIN/pi" "$FAKEBIN/agy"
ln -s "$(command -v git)" "$FAKEBIN/git"
ln -s "$OZ" "$FAKEBIN/oz"
set +e
(
  cd "$PROJECT"
  HOME="$TMP/home-missing-codex" XDG_STATE_HOME="$TMP/state-missing-codex" PATH="$FAKEBIN" "$WO" run --change "$CHANGE" --json
) >"$TMP/missing-codex.out" 2>"$TMP/missing-codex.err"
code=$?
set -e
cat "$TMP/missing-codex.out" "$TMP/missing-codex.err" | tee -a "$LOG"
[[ "$code" -ne 0 ]] || fail "缺少 codex 时 run 不应成功"
grep -Eiq 'codex' "$TMP/missing-codex.out" "$TMP/missing-codex.err" || fail "错误输出必须指出缺少 codex"
grep -Eiq '安装|install' "$TMP/missing-codex.out" "$TMP/missing-codex.err" || fail "错误输出必须提示安装 CLI"
assert_no_run_state "$TMP/state-missing-codex" "$TMP/missing-codex-state.txt"
printf 'missing codex: no run state created\n' >>"$STATE_SNAPSHOT"

note "第三后端配置必须按未知工具失败"
legacy_tool="open""code"
cat >"$PROJECT/oz-flow.yaml" <<YAML
wo:
  workflow:
    defaults:
      tool: ${legacy_tool}
YAML
set +e
(
  cd "$PROJECT"
  HOME="$TMP/home-invalid-tool" XDG_STATE_HOME="$TMP/state-invalid-tool" PATH="$TMP/bin-missing-pi" "$WO" run --change "$CHANGE" --json
) >"$TMP/invalid-tool.out" 2>"$TMP/invalid-tool.err"
code=$?
set -e
cat "$TMP/invalid-tool.out" "$TMP/invalid-tool.err" | tee -a "$LOG"
[[ "$code" -ne 0 ]] || fail "第三后端配置不应启动成功"
grep -Eiq '未知|unknown|agent tool|invalid' "$TMP/invalid-tool.out" "$TMP/invalid-tool.err" || fail "第三后端配置必须报告未知工具"

note "contract passed: startup preflight and backend validation are strict"
