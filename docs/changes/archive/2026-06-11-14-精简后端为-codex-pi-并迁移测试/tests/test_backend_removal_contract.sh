#!/usr/bin/env bash
# 文件功能目的：验证仓库彻底删除第三后端实现、文档承诺和测试夹具。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
LOG="$ROOT/test-results/14-backend-removal.log"
mkdir -p "$(dirname "$LOG")"
: >"$LOG"

# note 记录测试过程，让执行失败时可以直接从日志判断哪个业务契约未满足。
note() {
  printf '[backend-removal] %s\n' "$*" | tee -a "$LOG"
}

# fail 输出明确错误，避免执行器把语法错误误判成目标行为缺失。
fail() {
  printf '[backend-removal] FAIL: %s\n' "$*" | tee -a "$LOG" >&2
  exit 1
}

legacy_lower="open""code"
legacy_title="Open""Code"

note "扫描全仓库，提案自身通过动态拼词避免成为残留来源"
if hits="$(cd "$ROOT" && git grep -n -I -e "$legacy_lower" -e "$legacy_title" -- . ':(exclude).git' 2>/dev/null || true)" && [[ -n "$hits" ]]; then
  printf '%s\n' "$hits" | tee -a "$LOG"
  fail "仓库仍包含旧后端名称残留"
fi

note "确认旧后端实现文件已经删除"
if [[ -e "$ROOT/internal/app/${legacy_lower}.go" || -e "$ROOT/internal/app/${legacy_lower}_test.go" || -e "$ROOT/internal/app/${legacy_lower}_integration_test.go" ]]; then
  fail "internal/app 下仍存在旧后端实现或测试文件"
fi

note "确认 agent registry 没有注册第三个后端"
agent_file="$ROOT/internal/app/agent.go"
[[ -f "$agent_file" ]] || fail "缺少 internal/app/agent.go"
if grep -Eq 'Register\([^)]*(Code|code)|validAgentTool\(name string\).*Code|validAgentTool\(name string\).*code' "$agent_file"; then
  fail "agent registry 或工具白名单仍包含第三后端"
fi

note "contract passed: backend allowlist is codex/pi only"
