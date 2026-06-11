#!/usr/bin/env bash
# 文件功能目的：验证 agy runner 和 planning command 使用真实 CLI 参数数组，而不是复用 pi 或 shell 拼接。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
LOG="$ROOT/test-results/15-agy-cli-args.log"

note() {
  # 函数目的：记录 agy 参数映射契约检查步骤。
  printf '[agy-args] %s\n' "$*" | tee -a "$LOG"
}

fail() {
  # 函数目的：用明确错误终止契约测试。
  printf '[agy-args] FAIL: %s\n' "$*" | tee -a "$LOG" >&2
  exit 1
}

mkdir -p "$(dirname "$LOG")"
: >"$LOG"

note "源码必须提供独立 AgyTool 和 AgyCLI"
grep -R --include='*.go' -Eq 'type AgyTool struct|func \(AgyTool\) Name\(\) string \{ return "agy" \}' "$ROOT/internal" "$ROOT/tests" || fail "未找到独立 AgyTool"
grep -R --include='*.go' -Eq 'type AgyCLI struct|func .*agy.*Run\(' "$ROOT/internal" "$ROOT/tests" || fail "未找到 agy runner"

note "agy sealed run 必须使用 --print 和 --conversation"
grep -R --include='*.go' -Eq '"--print"|"-p"' "$ROOT/internal" "$ROOT/tests" || fail "agy sealed run 未声明 --print"
grep -R --include='*.go' -Eq '"--conversation"' "$ROOT/internal" "$ROOT/tests" || fail "agy resume 未映射 --conversation"

note "agy planning 必须使用 --prompt-interactive"
grep -R --include='*.go' -Eq '"--prompt-interactive"|"-i"' "$ROOT/internal" "$ROOT/tests" || fail "agy planning 未声明 --prompt-interactive"

note "agy 参数必须覆盖 model 和权限映射"
grep -R --include='*.go' -Eq '"--model"' "$ROOT/internal" "$ROOT/tests" || fail "agy 未映射 --model"
grep -R --include='*.go' -Eq '"--dangerously-skip-permissions"' "$ROOT/internal" "$ROOT/tests" || fail "agy 未映射危险权限参数"
grep -R --include='*.go' -Eq '"--sandbox"' "$ROOT/internal" "$ROOT/tests" || fail "agy 未映射 sandbox 参数"

note "长期测试必须覆盖 argv 级断言"
grep -R --include='*.go' --include='*.gotest' -Eq 'agy.*--print|--conversation.*conv-123|--prompt-interactive' "$ROOT/tests" || fail "缺少 agy argv 长期测试"

note "实现不得通过 shell 字符串拼接执行 agy"
if grep -R --include='*.go' -Eq 'sh -c.*agy|agy .*\\$\\{' "$ROOT/internal" "$ROOT/tests"; then
  fail "agy 调用疑似使用 shell 拼接"
fi
if grep -R --include='*.go' -Fq 'fmt.Sprintf("agy ' "$ROOT/internal" "$ROOT/tests"; then
  fail "agy 调用疑似使用 shell 拼接"
fi

note "contract passed: agy CLI argument mapping is covered"
