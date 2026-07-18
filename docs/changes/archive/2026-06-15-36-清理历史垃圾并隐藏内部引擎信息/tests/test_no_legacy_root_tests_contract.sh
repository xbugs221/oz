#!/usr/bin/env bash
# 文件功能目的：验证根目录历史 dated shell 测试已退出活跃维护面，不再携带旧 wo 产品合同。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/36-cleanup"
LOG="$RESULT_DIR/legacy-root-tests.log"

note() {
  # note 记录根测试层扫描过程，便于复核删除和迁移范围。
  printf '[legacy-root-tests] %s\n' "$*" | tee -a "$LOG"
}

fail() {
  # fail 输出根测试层仍不符合当前产品面的原因。
  note "FAIL: $*"
  exit 1
}

mkdir -p "$RESULT_DIR"
: >"$LOG"

cd "$ROOT"

note "确认当前 docs/changes/archive/2026-06-15-36-清理历史垃圾并隐藏内部引擎信息/tests/specs 仍作为业务合同入口存在"
[[ -d docs/changes/archive/2026-06-15-36-清理历史垃圾并隐藏内部引擎信息/tests/specs ]] || fail "缺少当前 specs 测试目录 docs/changes/archive/2026-06-15-36-清理历史垃圾并隐藏内部引擎信息/tests/specs"

note "根目录 tests 不应再保留 2026-* 历史 shell 测试"
dated_tests="$(fd -t f '^2026-.*\.sh$' tests 2>/dev/null || true)"
if [[ -n "$dated_tests" ]]; then
  printf '%s\n' "$dated_tests" | tee -a "$LOG"
  fail "根目录仍存在 dated 历史 shell 测试，应删除或迁移到当前测试层"
fi

note "扫描根测试层是否还引用旧 wo 产品面"
if [[ -d tests ]]; then
  root_hits="$(rg -n --glob '!tests/specs/**' \
    'cmd/wo|\.\/cmd/wo|wo\.yaml|(^|/)\.wo(/|$)|/wo/repos|XDG_STATE_HOME[^[:space:]]*/wo|\bwo (status|watch|run|clean|config|restart|resume|batch|abort|update|graph|contract|list-changes)\b|\bWO_TEST_|\bWO_PLANNING|\bWO=' \
    tests 2>/dev/null || true)"
  if [[ -n "$root_hits" ]]; then
    printf '%s\n' "$root_hits" | tee -a "$LOG"
    fail "根测试层仍引用旧 wo 命令、配置、状态目录或环境变量"
  fi
fi

note "PASS"
