#!/usr/bin/env bash
# 文件目的：验证 oz 和 wo 已经由同一个仓库、同一个发布批次构建，避免规范工具与执行器错位。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

log_dir="$repo_root/test-results/merge-wo"
mkdir -p "$log_dir"
log="$log_dir/monorepo-cli-release-contract.log"
: >"$log"

note() {
  printf '%s\n' "$*" | tee -a "$log"
}

fail() {
  note "FAIL: $*"
  exit 1
}

cd "$repo_root"

note "检查同一仓库内的 CLI 入口"
test -f "cmd/oz/main.go" || fail "缺少 cmd/oz/main.go"
test -f "cmd/wo/main.go" || fail "缺少 cmd/wo/main.go，wo 尚未合入当前仓库"
test -d "internal/app" || fail "缺少 internal/app，wo 执行器主体尚未合入当前仓库"
test -d "prompts-template" || fail "缺少 prompts-template，wo 内置提示词尚未合入当前仓库"

module="$(go list -m)"
note "go module: $module"
test "$module" = "github.com/xbugs221/oz" || fail "合并后模块名必须保持 github.com/xbugs221/oz"

app_import="$(go list -f '{{.ImportPath}}' ./internal/app)"
note "internal/app import path: $app_import"
test "$app_import" = "github.com/xbugs221/oz/internal/app" || fail "internal/app 仍未归入 oz module path"

if go list -deps ./cmd/wo | grep -q '^github.com/xbugs221/wo'; then
  fail "cmd/wo 依赖中仍出现 github.com/xbugs221/wo"
fi
note "cmd/wo deps 不含旧模块路径"

note "构建 oz 和 wo"
go build -o "$tmp/oz" ./cmd/oz 2>&1 | tee -a "$log"
go build -o "$tmp/wo" ./cmd/wo 2>&1 | tee -a "$log"
"$tmp/oz" --version >"$tmp/oz.version"
"$tmp/wo" --version >"$tmp/wo.version"
note "oz version: $(cat "$tmp/oz.version")"
note "wo version: $(cat "$tmp/wo.version")"

note "检查 CI/Release workflow 不再下载外部 latest oz"
workflow_dir="$repo_root/.github/workflows"
test -d "$workflow_dir" || fail "缺少 .github/workflows"
shopt -s nullglob
workflows=("$workflow_dir"/*.yml "$workflow_dir"/*.yaml)
test "${#workflows[@]}" -gt 0 || fail "缺少 GitHub Actions workflow"

workflow_text="$tmp/workflows.txt"
cat "${workflows[@]}" >"$workflow_text"
if grep -q 'github.com/xbugs221/oz/releases/latest/download' "$workflow_text"; then
  fail "workflow 仍从 GitHub latest 下载外部 oz"
fi
grep -q './cmd/oz' "$workflow_text" || fail "workflow 未体现从当前 checkout 构建 cmd/oz"
grep -q './cmd/wo' "$workflow_text" || fail "workflow 未体现从当前 checkout 构建 cmd/wo"
grep -q 'go test ./...' "$workflow_text" || fail "workflow 未保留 go test ./... 门禁"

note "PASS"
