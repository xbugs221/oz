#!/usr/bin/env bash
# 文件功能目的：验证工作流能力由唯一 oz 二进制承载，并通过 oz flow 命令组访问。
# Sources: 30-彻底合并wo为oz-flow
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

note() {
  # note 记录合同验证步骤，方便定位失败的业务断言。
  printf '%s\n' "$*"
}

fail() {
  # fail 输出业务合同失败原因并终止测试。
  note "FAIL: $*"
  exit 1
}

cd "$repo_root"

[[ -f cmd/oz/main.go ]] || fail "缺少唯一 CLI 入口 cmd/oz/main.go"
[[ ! -e cmd/wo ]] || fail "仍存在独立 wo CLI 入口 cmd/wo"

note "构建唯一 oz 二进制"
go build -o "$tmp/oz" ./cmd/oz
[[ -x "$tmp/oz" ]] || fail "oz 二进制未生成"

note "检查 oz 帮助展示 flow 命令组"
"$tmp/oz" --help >"$tmp/oz-help.txt" 2>&1
grep -qE '(^|[[:space:]])flow([[:space:]]|$)' "$tmp/oz-help.txt" || fail "oz --help 未展示 flow 命令组"

note "检查 oz flow 帮助和只读状态入口可执行"
"$tmp/oz" flow --help >"$tmp/flow-help.txt" 2>&1
grep -qE 'status|watch|run|config' "$tmp/flow-help.txt" || fail "oz flow --help 未展示工作流子命令"

flow_repo="$tmp/flow-repo"
mkdir -p "$flow_repo"
git -C "$flow_repo" init -q
(
  cd "$flow_repo"
  "$tmp/oz" flow status >"$tmp/flow-status.txt" 2>&1
)
if grep -qE '(^|[[:space:]])wo([[:space:]]|$)|wo clean|wo restart|wo status' "$tmp/flow-status.txt"; then
  fail "oz flow status 输出仍提示旧 wo 命令"
fi

(
  cd "$flow_repo"
  timeout -s INT 2s "$tmp/oz" flow watch >"$tmp/flow-watch.txt" 2>&1 || true
)
if grep -qE '(^|[[:space:]])wo([[:space:]]|$)|wo clean|wo restart|wo watch' "$tmp/flow-watch.txt"; then
  fail "oz flow watch 输出仍提示旧 wo 命令"
fi

note "检查 CI/Release 不再构建独立 wo"
workflow_dir="$repo_root/.github/workflows"
[[ -d "$workflow_dir" ]] || fail "缺少 GitHub Actions workflow 目录"
if rg -n 'cmd/wo|./cmd/wo|/wo("|$|[[:space:]])|go install .*cmd/wo' "$workflow_dir"; then
  fail "workflow 仍引用独立 wo 构建或发布产物"
fi

note "运行 Go 全量回归"
go test ./...

note "PASS"
