#!/usr/bin/env bash
# 文件功能目的：验证 agy 可作为 Pi 候选后端配置，并纳入 sealed run 启动前预检。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
TMP="$ROOT/.tmp/15-agy-config-preflight"
LOG="$ROOT/test-results/15-agy-config-preflight.log"
STATE_SNAPSHOT="$ROOT/test-results/15-agy-state-snapshot.txt"

note() {
  # 函数目的：把业务步骤同时写到终端和验收日志，方便执行阶段复查。
  printf '[agy-config] %s\n' "$*" | tee -a "$LOG"
}

fail() {
  # 函数目的：输出明确失败原因，并让契约测试以非零状态退出。
  printf '[agy-config] FAIL: %s\n' "$*" | tee -a "$LOG" >&2
  exit 1
}

write_fake_cli() {
  # 函数目的：创建只满足预检的 fake agent CLI，避免测试依赖真实账号。
  local path="$1"
  local name="$2"
  cat >"$path/$name" <<'SH'
#!/usr/bin/env bash
printf '{"type":"session","id":"fake-session"}\n'
SH
  chmod +x "$path/$name"
}

prepare_change() {
  # 函数目的：创建最小真实 oz change，让 wo run 走真实配置读取和预检入口。
  local repo="$1"
  local change="$repo/docs/changes/99-agy候选契约"
  mkdir -p "$change/tests"
  cat >"$change/proposal.md" <<'MD'
# 提案：agy contract
MD
  cat >"$change/design.md" <<'MD'
# 设计：agy contract
MD
  cat >"$change/spec.md" <<'MD'
# 规格：agy 候选契约

### 需求：验证 agy 候选

系统必须提供一个可运行的临时契约。

#### 场景：读取 brief

- **给定** 临时 change 已创建 brief
- **当** 执行契约测试
- **则** 测试必须能读取 brief
MD
  cat >"$change/task.md" <<'MD'
# 任务：agy contract

- [ ] 1.1 运行临时契约测试。
MD
  cat >"$change/tests/test_smoke.sh" <<'SH'
#!/usr/bin/env bash
# 文件功能目的：提供临时 change 的最小真实契约测试。
set -euo pipefail
test -f docs/changes/99-agy候选契约/brief.md
SH
  chmod +x "$change/tests/test_smoke.sh"
  cat >"$change/brief.md" <<'MD'
# 简报：agy contract
MD
  cat >"$change/acceptance.json" <<'JSON'
{
  "summary": "agy contract",
  "coverage": [
    {
      "spec": "需求：验证 agy 候选 / 场景：读取 brief",
      "tests": ["smoke"],
      "evidence": [],
      "risk": "临时 change 只用于触发真实 wo run 入口。"
    }
  ],
  "required_tests": [
    {
      "id": "smoke",
      "source": "change_contract",
      "path": "docs/changes/99-agy候选契约/tests/test_smoke.sh",
      "command": "bash docs/changes/99-agy候选契约/tests/test_smoke.sh",
      "purpose": "证明临时 change 有真实测试入口。",
      "assertions": ["brief 文件存在"]
    }
  ],
  "required_evidence": []
}
JSON
  cat >"$repo/wo.yaml" <<'YAML'
wo:
  workflow:
    stages:
      execution:
        tool: agy
        model: agy-model
        permissions: danger-full-access
    parallel:
      enabled: true
      groups:
        implementation_context:
          mode: advisory
          members:
            - name: "agy候选测试员"
              purpose: "验证 agy 可作为 pi 候选后端"
              stage: before_execution
              tool: agy
YAML
}

rm -rf "$TMP"
mkdir -p "$TMP/bin" "$(dirname "$LOG")"
: >"$LOG"
: >"$STATE_SNAPSHOT"

note "编译真实 wo/oz CLI"
WO="$TMP/wo"
OZ="$TMP/oz"
(cd "$ROOT" && go build -o "$WO" ./cmd/wo) 2>&1 | tee -a "$LOG"
(cd "$ROOT" && go build -o "$OZ" ./cmd/oz) 2>&1 | tee -a "$LOG"

note "缺少 agy 时必须在创建运行态前失败"
write_fake_cli "$TMP/bin" codex
write_fake_cli "$TMP/bin" pi
ln -sf "$(command -v git)" "$TMP/bin/git"
ln -sf "$OZ" "$TMP/bin/oz"
REPO_MISSING="$TMP/repo-missing-agy"
mkdir -p "$REPO_MISSING"
(cd "$REPO_MISSING" && git init -q && git config user.email test@example.com && git config user.name Test)
prepare_change "$REPO_MISSING"
(cd "$REPO_MISSING" && git add . && git commit -q -m init)
set +e
(
  cd "$REPO_MISSING"
  HOME="$TMP/home-missing" XDG_STATE_HOME="$TMP/state-missing" PATH="$TMP/bin" "$WO" run --change 99-agy候选契约 --json
) >"$TMP/missing-agy.out" 2>"$TMP/missing-agy.err"
code=$?
set -e
cat "$TMP/missing-agy.out" "$TMP/missing-agy.err" | tee -a "$LOG"
[[ "$code" -ne 0 ]] || fail "缺少 agy 时 run 不应成功"
grep -Eiq 'agy' "$TMP/missing-agy.out" "$TMP/missing-agy.err" || fail "错误输出必须指出缺少 agy"
grep -Eiq '安装|install' "$TMP/missing-agy.out" "$TMP/missing-agy.err" || fail "错误输出必须提示用户先安装 CLI"
if compgen -G "$TMP/state-missing/oz/runs/*/state.json" >/dev/null; then
  fail "缺少 agy 时不应创建 run state"
fi
if compgen -G "$TMP/state-missing/wo/repos/*/runs/*/state.json" >/dev/null; then
  fail "缺少 agy 时不应创建 wo run state"
fi
printf 'missing agy: no run state created\n' >>"$STATE_SNAPSHOT"

note "配置层必须接受 agy stage 和 agy parallel member"
write_fake_cli "$TMP/bin" agy
REPO_WITH_AGY="$TMP/repo-with-agy"
mkdir -p "$REPO_WITH_AGY"
(cd "$REPO_WITH_AGY" && git init -q && git config user.email test@example.com && git config user.name Test)
prepare_change "$REPO_WITH_AGY"
(cd "$REPO_WITH_AGY" && git add . && git commit -q -m init)
set +e
(
  cd "$REPO_WITH_AGY"
  HOME="$TMP/home-with" XDG_STATE_HOME="$TMP/state-with" PATH="$TMP/bin" "$WO" run --change 99-agy候选契约 --json
) >"$TMP/with-agy.out" 2>"$TMP/with-agy.err"
code=$?
set -e
cat "$TMP/with-agy.out" "$TMP/with-agy.err" | tee -a "$LOG"
if grep -Eiq '未知 agent tool "agy"|unknown agent tool "agy"' "$TMP/with-agy.out" "$TMP/with-agy.err"; then
  fail "配置读取仍把 agy 当成未知工具"
fi

note "源码和状态展示必须使用 agy session key，而不是把 agy 映射成 pi"
grep -R --include='*.go' -Eq 'sessionStateKey\("agy"|Name\(\) string \{ return "agy" \}' "$ROOT/internal" "$ROOT/tests" || fail "缺少 agy 独立 tool 或 agy session key 覆盖"
if grep -R --include='*.go' -Eq 'agy.*return "pi"|case "agy".*"pi"' "$ROOT/internal" "$ROOT/tests"; then
  fail "agy 不得作为 pi 别名实现"
fi
printf 'agy key: independent from pi\n' >>"$STATE_SNAPSHOT"

note "contract passed: agy config and preflight behavior are covered"
