#!/usr/bin/env bash
# 文件功能目的：验证迁移测试层已收敛，根测试门禁代表当前真实业务合同。
# Sources: 14-精简后端为-codex-pi-并迁移测试, 20-收敛迁移测试合同
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
LOG="$ROOT/test-results/spec-root-test-layout.log"
mkdir -p "$(dirname "$LOG")"
: >"$LOG"

# note 写入可审计日志，说明测试布局检查的真实结果。
note() {
  printf '[root-test-layout] %s\n' "$*" | tee -a "$LOG"
}

# fail 在布局不符合约定时直接失败。
fail() {
  printf '[root-test-layout] FAIL: %s\n' "$*" | tee -a "$LOG" >&2
  exit 1
}

note "检查根目录 tests 存在业务测试入口"
[[ -d "$ROOT/tests" ]] || fail "缺少根目录 tests 目录"
root_test_count="$(cd "$ROOT" && find tests -type f \( -name '*_test.go' -o -name '*.gotest' -o -name '*.sh' \) | wc -l | tr -d ' ')"
[[ "$root_test_count" -gt 0 ]] || fail "根目录 tests 下没有可运行测试入口"

note "检查 tests/app 不再保留 .gotest 迁移输入"
if app_gotests="$(cd "$ROOT" && find tests/app -type f -name '*.gotest' | sort)" && [[ -n "$app_gotests" ]]; then
  printf '%s\n' "$app_gotests" | tee -a "$LOG"
  fail "tests/app 仍存在 .gotest 迁移测试输入"
fi

note "检查 tests/app 或 tests/specs 至少存在一个分组"
if [[ ! -d "$ROOT/tests/app" && ! -d "$ROOT/tests/specs" ]]; then
  fail "根目录 tests 缺少 app 或 specs 分组"
fi

note "检查核心真实测试包可单独运行"
(cd "$ROOT" && go test ./internal/app ./cmd/oz ./tests -count=1) 2>&1 | tee -a "$LOG" || fail "核心真实测试包未通过"

note "检查根目录 Go 测试门禁稳定"
(cd "$ROOT" && go test ./... -count=1) 2>&1 | tee -a "$LOG" || fail "go test ./... 未通过"

note "contract passed: migrated app tests no longer hide inside root gate"
