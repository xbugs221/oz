#!/usr/bin/env bash
# Sources: 2-合并-wo-执行器到-oz-仓库
# 文件目的：验证 oz 和 wo 由同一个仓库、同一个发布批次构建。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

cd "$repo_root"

test -f "cmd/oz/main.go"
test -f "cmd/wo/main.go"
test -d "internal/app"
test -d "prompts-template"

module="$(go list -m)"
test "$module" = "github.com/xbugs221/oz"

app_import="$(go list -f '{{.ImportPath}}' ./internal/app)"
test "$app_import" = "github.com/xbugs221/oz/internal/app"

if go list -deps ./cmd/wo | grep -q '^github.com/xbugs221/wo'; then
  echo "cmd/wo 依赖中仍出现 github.com/xbugs221/wo" >&2
  exit 1
fi

go build -o "$tmp/oz" ./cmd/oz
go build -o "$tmp/wo" ./cmd/wo
"$tmp/oz" --version >/dev/null
"$tmp/wo" --version >/dev/null

workflow_dir="$repo_root/.github/workflows"
test -d "$workflow_dir"
shopt -s nullglob
workflows=("$workflow_dir"/*.yml "$workflow_dir"/*.yaml)
test "${#workflows[@]}" -gt 0

workflow_text="$tmp/workflows.txt"
cat "${workflows[@]}" >"$workflow_text"
if grep -q 'github.com/xbugs221/oz/releases/latest/download' "$workflow_text"; then
  echo "workflow 仍从 GitHub latest 下载外部 oz" >&2
  exit 1
fi
grep -q './cmd/oz' "$workflow_text"
grep -q './cmd/wo' "$workflow_text"
grep -q 'go test ./...' "$workflow_text"

echo "PASS"
