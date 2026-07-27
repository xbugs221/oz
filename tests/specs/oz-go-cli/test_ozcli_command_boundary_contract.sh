#!/usr/bin/env bash
# 文件功能目的：长期验证 standalone oz CLI 按命令职责拆分源码文件并保持回归通过。
# Sources: 34-拆分ozcli命令边界, 47-移除并禁止提案任务文件
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
log="$repo_root/test-results/oz-go-cli/ozcli-command-boundary-contract.log"
mkdir -p "$(dirname "$log")"
: >"$log"

note() {
  # note 记录结构检查和回归命令，作为 ozcli-command-boundary-log 运行证据。
  printf '%s\n' "$*" | tee -a "$log"
}

fail() {
  # fail 用清晰的边界原因终止合同测试。
  note "FAIL: $*"
  exit 1
}

assert_file_has() {
  # assert_file_has 确认目标源码文件存在并包含指定职责函数。
  local file="$1"
  local pattern="$2"
  [[ -f "$file" ]] || fail "缺少目标文件：$file"
  rg -n "$pattern" "$file" >>"$log" || fail "$file 缺少模式：$pattern"
}

assert_file_lacks() {
  # assert_file_lacks 确认已移除的职责不会重新进入目标源码文件。
  local file="$1"
  local pattern="$2"
  [[ -f "$file" ]] || fail "缺少目标文件：$file"
  if rg -n "$pattern" "$file" >>"$log"; then
    fail "$file 不应包含模式：$pattern"
  fi
}

cd "$repo_root"

note "evidence id: ozcli-command-boundary-log"
note "evidence path: $log"
note "test id: ozcli-command-boundary-contract"

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
assert_file_has "internal/ozcli/cmd_validate.go" 'active 提案禁止包含 task.md'
assert_file_lacks "internal/ozcli/cmd_archive.go" 'ensureTasksDone'
assert_file_lacks "internal/ozcli/cmd_archive.go" 'task.md'

if [[ -f internal/ozcli/ozcli.go ]]; then
  line_count="$(wc -l < internal/ozcli/ozcli.go | tr -d ' ')"
  note "ozcli.go line_count=$line_count"
  (( line_count <= 180 )) || fail "ozcli.go 仍然过大，说明命令职责没有真正拆分"
fi

note "运行 internal/ozcli Go 回归"
go test ./internal/ozcli -count=1 2>&1 | tee -a "$log"

note "PASS: ozcli-command-boundary-contract"
