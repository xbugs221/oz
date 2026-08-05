#!/usr/bin/env bash
# 文件功能目的：验证工作流后端集合稳定收敛为 Codex/Pi/Agy/Claude，不再保留历史第三后端源码、文档承诺或状态 key。
# Sources: 14-精简后端为-codex-pi-并迁移测试, 15-支持-agy-cli作为pi候选, claude-code-backend
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
LOG="$ROOT/test-results/spec-agent-backend-allowlist.log"
mkdir -p "$(dirname "$LOG")"
: >"$LOG"

# note 记录仓库扫描过程，便于复核旧后端残留来自哪里。
note() {
  printf '[agent-backend-allowlist] %s\n' "$*" | tee -a "$LOG"
}

# fail 输出明确失败原因，避免把扫描命令异常误判为合同通过。
fail() {
  printf '[agent-backend-allowlist] FAIL: %s\n' "$*" | tee -a "$LOG" >&2
  exit 1
}

legacy_lower="open""code"
legacy_title="Open""Code"

note "扫描源码、主规格和根目录长期测试，确认第三后端名称不再出现"
if hits="$(cd "$ROOT" && git grep -n -I -e "$legacy_lower" -e "$legacy_title" -- \
  internal cmd docs/specs tests \
  ':(exclude)tests/specs/codex-workflow-cli/test_agent_backend_allowlist_contract.sh' \
  ':(exclude)tests/specs/codex-workflow-cli/test_user_visible_surface_cleanup_contract.sh' 2>/dev/null || true)" && [[ -n "$hits" ]]; then
  printf '%s\n' "$hits" | tee -a "$LOG"
  fail "当前源码、主规格或长期测试仍包含第三后端残留"
fi

note "确认第三后端实现文件不存在"
if [[ -e "$ROOT/internal/app/${legacy_lower}.go" || -e "$ROOT/internal/app/${legacy_lower}_test.go" || -e "$ROOT/internal/app/${legacy_lower}_integration_test.go" ]]; then
  fail "internal/app 下仍存在第三后端实现或测试文件"
fi

note "确认 agent registry 只注册并接受 codex/pi/agy/claude"
agent_file="$ROOT/internal/app/agent.go"
[[ -f "$agent_file" ]] || fail "缺少 internal/app/agent.go"
grep -q 'return name == "codex" || name == "pi" || name == "agy" || name == "claude"' "$agent_file" || fail "validAgentTool 未明确收敛为 codex/pi/agy/claude"
for tool in CodexTool PiTool AgyTool ClaudeTool; do
  grep -q "$tool{}" "$agent_file" || fail "NewAgentRegistry 缺少 $tool"
done
go test "$ROOT/internal/app" -run 'TestAgentRegistryResolvesConfiguredTools|TestValidAgentTool' -count=1 >>"$LOG" 2>&1

note "contract passed: agent backend allowlist is codex/pi/agy/claude only"
