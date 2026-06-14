#!/usr/bin/env bash
# 文件功能：验证 standalone oz CLI 已按命令职责拆分并保持 Go 回归通过。

set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

EVIDENCE="test-results/34-ozcli-boundary/contract.log"
mkdir -p "$(dirname "$EVIDENCE")"
: > "$EVIDENCE"

note() {
  printf '%s\n' "$*" | tee -a "$EVIDENCE"
}

fail() {
  note "FAIL: $*"
  exit 1
}

assert_file_has() {
  local file="$1"
  local pattern="$2"
  [[ -f "$file" ]] || fail "缺少目标文件：$file"
  rg -n "$pattern" "$file" >>"$EVIDENCE" || fail "$file 缺少模式：$pattern"
}

note "ozcli-boundary-log: 检查 ozcli 命令文件边界"
assert_file_has "internal/ozcli/cli.go" 'func Main'
assert_file_has "internal/ozcli/cli.go" 'func \(c \*cli\) run'
assert_file_has "internal/ozcli/cmd_install.go" 'func \(c \*cli\) installCmd'
assert_file_has "internal/ozcli/cmd_install.go" 'func \(c \*cli\) printInstallHelp'
assert_file_has "internal/ozcli/cmd_change.go" 'func \(c \*cli\) listCmd'
assert_file_has "internal/ozcli/cmd_change.go" 'func \(c \*cli\) createCmd'
assert_file_has "internal/ozcli/cmd_change.go" 'func \(c \*cli\) statusCmd'
assert_file_has "internal/ozcli/cmd_validate.go" 'func \(c \*cli\) validateCmd'
assert_file_has "internal/ozcli/cmd_validate.go" 'func validateChange'
assert_file_has "internal/ozcli/cmd_validate.go" 'func validateAcceptanceFiles'
assert_file_has "internal/ozcli/cmd_archive.go" 'func \(c \*cli\) archiveCmd'
assert_file_has "internal/ozcli/cmd_archive.go" 'func ensureTasksDone'

if [[ -f internal/ozcli/ozcli.go ]]; then
  line_count="$(wc -l < internal/ozcli/ozcli.go | tr -d ' ')"
  note "ozcli.go line_count=$line_count"
  (( line_count <= 180 )) || fail "ozcli.go 仍然过大，说明命令职责没有真正拆分"
fi

note "运行 internal/ozcli Go 回归"
go test ./internal/ozcli -count=1 | tee -a "$EVIDENCE"

note "contract passed: ozcli 命令边界已拆分，证据位于 $EVIDENCE"
