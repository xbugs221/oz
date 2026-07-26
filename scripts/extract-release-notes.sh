#!/usr/bin/env bash
# 文件功能目的：从 CHANGELOG.md 提取指定版本或“尚未发布”内容，供 GitHub Release 复用。

set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "用法: $0 <CHANGELOG.md> <版本标签> <输出文件>" >&2
  exit 2
fi

changelog_path="$1"
version="$2"
output_path="$3"
notes_tmp="$(mktemp)"
trap 'rm -f "$notes_tmp"' EXIT

# extract_section 提取一个二级版本标题下、下一个二级版本标题前的正文。
extract_section() {
  local heading="$1"
  awk -v heading="$heading" '
    $0 == heading || index($0, heading " - ") == 1 { found = 1; next }
    found && /^## \[/ { exit }
    found { print }
  ' "$changelog_path"
}

extract_section "## [$version]" >"$notes_tmp"
if ! grep -q '[^[:space:]]' "$notes_tmp"; then
  extract_section "## [尚未发布]" >"$notes_tmp"
fi
if ! grep -q '[^[:space:]]' "$notes_tmp"; then
  echo "CHANGELOG.md 缺少 [$version] 或 [尚未发布] 的有效发布说明" >&2
  exit 1
fi

mv "$notes_tmp" "$output_path"
trap - EXIT
