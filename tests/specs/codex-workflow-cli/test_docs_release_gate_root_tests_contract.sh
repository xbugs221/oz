#!/usr/bin/env bash
# 文件功能目的：验证主规格和发布门禁只承诺 Codex/Pi 后端以及 Go 测试入口。
# Sources: 14-精简后端为-codex-pi-并迁移测试
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
LOG="$ROOT/test-results/spec-docs-release-gate-root-tests.log"
mkdir -p "$(dirname "$LOG")"
: >"$LOG"

# note 记录文档和发布门禁检查过程。
note() {
  printf '[docs-release-gate-root-tests] %s\n' "$*" | tee -a "$LOG"
}

# fail 在文档或门禁合同不匹配时失败。
fail() {
  printf '[docs-release-gate-root-tests] FAIL: %s\n' "$*" | tee -a "$LOG" >&2
  exit 1
}

SPEC="$ROOT/docs/specs/codex-workflow-cli/spec.md"
RELEASE_SPEC="$ROOT/docs/specs/release-automation/spec.md"
legacy_lower="open""code"
legacy_title="Open""Code"

[[ -f "$SPEC" ]] || fail "缺少主规格 $SPEC"
[[ -f "$RELEASE_SPEC" ]] || fail "缺少发布规格 $RELEASE_SPEC"

note "主规格不得再承诺第三后端"
if grep -n -E "$legacy_lower|$legacy_title|old-agent|OldAgent" "$SPEC" | tee -a "$LOG"; then
  fail "主规格仍包含第三后端描述"
fi
if grep -n -E 'codex.*pi.*old-agent|codex.*old-agent.*pi|old-agent.*codex.*pi' "$SPEC" | tee -a "$LOG"; then
  fail "主规格仍包含三后端枚举"
fi

note "主规格必须明确只支持 codex 和 pi"
grep -Eiq '只支持.*codex.*pi|codex.*pi.*两个|codex.*和.*pi|Codex.*Pi' "$SPEC" || fail "主规格没有明确后端集合为 codex/pi"

note "主规格必须包含启动前检查 codex/pi 的要求"
grep -Eiq '启动前.*codex.*pi|Codex.*Pi.*(存在|安装|PATH)|codex.*pi.*(存在|安装|PATH)' "$SPEC" || fail "主规格没有声明启动前检查两个 CLI"

note "发布门禁必须使用 Go 全量测试入口"
grep -q 'go test ./...' "$RELEASE_SPEC" || fail "发布规格没有引用 go test ./..."
if grep -n 'go test ./internal' "$RELEASE_SPEC" | tee -a "$LOG"; then
  fail "发布门禁不应依赖 internal 同包测试入口"
fi

note "contract passed: docs and release gate match codex/pi go-test layout"
