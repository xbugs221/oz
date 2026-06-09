#!/usr/bin/env bash
# 验证 wo config 只写 YAML 配置，并且 prompt 来源优先使用 YAML。
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

WO="$TMP/wo"
(cd "$ROOT" && go build -o "$WO" ./cmd/wo)
export HOME="$TMP/home"
export XDG_STATE_HOME="$TMP/state"
mkdir -p "$HOME"

REPO="$TMP/repo"
mkdir -p "$REPO"
git -C "$REPO" init -q
git -C "$REPO" config user.email test@example.com
git -C "$REPO" config user.name Test
touch "$REPO/README.md"
git -C "$REPO" add README.md
git -C "$REPO" commit -q -m init

(cd "$REPO" && "$WO" config >"$TMP/wo-config.out")
test -f "$REPO/wo.yaml"
test ! -e "$REPO/.wo"
grep -q "planning:" "$REPO/wo.yaml"
grep -q "execution:" "$REPO/wo.yaml"
grep -q "fix:" "$REPO/wo.yaml"
grep -q "prompts:" "$REPO/wo.yaml"

NON_GIT="$TMP/non-git"
mkdir -p "$NON_GIT"
(cd "$NON_GIT" && "$WO" config --global >"$TMP/wo-config-global.out")
test -f "$HOME/wo.yaml"
test ! -e "$HOME/.wo"

set +e
(cd "$REPO" && "$WO" init) >"$TMP/wo-init.out" 2>&1
INIT_STATUS=$?
(cd "$REPO" && "$WO" install) >"$TMP/wo-install.out" 2>&1
INSTALL_STATUS=$?
set -e
test "$INIT_STATUS" -ne 0
test "$INSTALL_STATUS" -ne 0
grep -q "wo config" "$TMP/wo-init.out"
grep -q "prompt 已内嵌" "$TMP/wo-install.out"
