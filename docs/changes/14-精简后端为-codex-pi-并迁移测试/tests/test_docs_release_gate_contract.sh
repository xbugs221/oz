#!/usr/bin/env bash
# 文件功能目的：验证主规格和发布门禁只承诺 Codex/Pi 后端以及根目录测试布局。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
LOG="$ROOT/test-results/14-docs-release-gate.log"
mkdir -p "$(dirname "$LOG")"
: >"$LOG"

# note 记录文档和发布门禁检查过程。
note() {
  printf '[docs-release-gate] %s\n' "$*" | tee -a "$LOG"
}

# fail 在文档或门禁合同不匹配时失败。
fail() {
  printf '[docs-release-gate] FAIL: %s\n' "$*" | tee -a "$LOG" >&2
  exit 1
}

SPEC="$ROOT/docs/specs/codex-workflow-cli/spec.md"
RELEASE_TEST="$ROOT/tests/2026-05-15-20-修复-cicd-并强化测试门禁-test_release_workflow_runs_business_tests.sh"

[[ -f "$SPEC" ]] || fail "缺少主规格 $SPEC"
[[ -f "$RELEASE_TEST" ]] || fail "缺少发布门禁测试 $RELEASE_TEST"

legacy_lower="open""code"
legacy_title="Open""Code"

note "主规格不得再承诺第三后端"
if grep -n -E "$legacy_lower|$legacy_title" "$SPEC" | tee -a "$LOG"; then
  fail "主规格仍包含第三后端描述"
fi

note "主规格必须明确只支持 codex 和 pi"
grep -Eiq '只支持.*codex.*pi|codex.*pi.*两个|codex.*和.*pi' "$SPEC" || fail "主规格没有明确后端集合为 codex/pi"

note "主规格必须包含启动前检查 codex/pi 的要求"
grep -Eiq '启动前.*codex.*pi|codex.*pi.*(存在|安装|PATH)' "$SPEC" || fail "主规格没有声明启动前检查两个 CLI"

note "发布门禁必须以根目录 tests 为入口"
grep -q 'tests/' "$RELEASE_TEST" || fail "发布门禁测试没有引用根目录 tests"
if grep -n 'go test ./internal' "$RELEASE_TEST" | tee -a "$LOG"; then
  fail "发布门禁不应依赖 internal 同包测试入口"
fi

note "contract passed: docs and release gate match codex/pi root-test layout"
