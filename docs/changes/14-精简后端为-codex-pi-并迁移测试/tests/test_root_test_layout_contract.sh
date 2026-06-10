#!/usr/bin/env bash
# 文件功能目的：验证生产源码目录不再保存长期测试，测试入口集中在根目录 tests。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
LOG="$ROOT/test-results/14-root-test-layout.log"
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

note "扫描 internal 目录下的 Go 测试文件"
if internal_tests="$(cd "$ROOT" && find internal -type f -name '*_test.go' | sort)" && [[ -n "$internal_tests" ]]; then
  printf '%s\n' "$internal_tests" | tee -a "$LOG"
  fail "internal 目录仍包含长期 Go 测试文件"
fi

note "检查根目录 tests 存在业务测试入口"
[[ -d "$ROOT/tests" ]] || fail "缺少根目录 tests 目录"
root_test_count="$(cd "$ROOT" && find tests -type f \( -name '*_test.go' -o -name '*.sh' \) | wc -l | tr -d ' ')"
[[ "$root_test_count" -gt 0 ]] || fail "根目录 tests 下没有可运行测试入口"

note "检查 tests/app 或 tests/specs 至少存在一个分组"
if [[ ! -d "$ROOT/tests/app" && ! -d "$ROOT/tests/specs" ]]; then
  fail "根目录 tests 缺少 app 或 specs 分组"
fi

note "contract passed: source and tests are physically separated"
