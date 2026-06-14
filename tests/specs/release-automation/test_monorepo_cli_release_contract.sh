#!/usr/bin/env bash
# Sources: 2-合并-oz-flow-执行器到-oz-仓库, 18-修复GitHub-CI并更新仓库文档
# 文件目的：验证 oz 和 oz flow 由同一个仓库、同一个发布批次构建，并约束 CI/Release 文档门禁一致。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

cd "$repo_root"

test -f "cmd/oz/main.go"
test -f "cmd/oz/main.go"
test -d "internal/app"
test -d "prompts-template"
test -f "README.md"
test -f "docs/specs/release-automation/spec.md"

module="$(go list -m)"
test "$module" = "github.com/xbugs221/oz"

app_import="$(go list -f '{{.ImportPath}}' ./internal/app)"
test "$app_import" = "github.com/xbugs221/oz/internal/app"

old_workflow_module="github.com/xbugs221/""wo"
if go list -deps ./cmd/oz | grep -q "^$old_workflow_module"; then
  echo "cmd/oz 依赖中仍出现迁移前的旧工作流模块路径" >&2
  exit 1
fi

go build -o "$tmp/oz" ./cmd/oz
"$tmp/oz" --version >/dev/null
"$tmp/oz" flow --help >/dev/null

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
grep -q 'go test ./...' "$workflow_text"
if grep -q 'for script in tests/\*\.sh' "$workflow_text"; then
  echo "workflow 仍盲目遍历历史根目录 shell 脚本" >&2
  exit 1
fi

grep -q 'GitHub Actions' README.md
grep -q 'CI' README.md
grep -q 'Release' README.md
grep -q 'go test ./...' README.md
grep -Eq '本地复现|失败排查|复现 GitHub' README.md

grep -q 'go test ./...' docs/specs/release-automation/spec.md
grep -Eq 'CI 和 Release 使用本地 oz|本地 `oz`' docs/specs/release-automation/spec.md

echo "PASS"
