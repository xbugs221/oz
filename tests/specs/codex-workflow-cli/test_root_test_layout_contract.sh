#!/usr/bin/env bash
# 文件功能目的：验证迁移测试层已收敛，根测试门禁代表当前真实业务合同。
# Sources: 14-精简后端为-codex-pi-并迁移测试, 20-收敛迁移测试合同, 35-迁移动态gotest合同
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
LOG="$ROOT/test-results/spec-root-test-layout.log"
mkdir -p "$(dirname "$LOG")"
: >"$LOG"

# note 写入可审计日志，说明测试布局检查的真实结果。
note() {
  printf '[root-test-layout] %s\n' "$*" | tee -a "$LOG"
}

# fail 在布局不符合约定时直接失败。
fail() {
  printf '[root-test-layout] FAIL: %s\n' "$*" | tee -a "$LOG" >&2
  exit 1
}

note "检查根目录 tests 存在业务测试入口"
[[ -d "$ROOT/tests" ]] || fail "缺少根目录 tests 目录"
root_test_count="$(cd "$ROOT" && { fd -e go -e sh . tests 2>/dev/null || true; } | wc -l | tr -d ' ')"
[[ "$root_test_count" -gt 0 ]] || fail "根目录 tests 下没有可运行测试入口"

note "检查 tests/app 不再保留迁移输入目录"
if [[ -d "$ROOT/tests/app" ]] && app_inputs="$(cd "$ROOT" && { fd -t f . tests/app 2>/dev/null || true; } | sort)" && [[ -n "$app_inputs" ]]; then
  printf '%s\n' "$app_inputs" | tee -a "$LOG"
  fail "tests/app 仍存在迁移测试输入"
fi

note "检查动态 gotest runner 入口已清除"
migrated_runner_path="$ROOT/tests/app/migrated_app_""suite_test.go"
if [[ -f "$migrated_runner_path" ]]; then
  fail "tests/app/migrated_app_""suite_test.go 仍存在，动态 runner 尚未删除"
fi
dynamic_runner_pattern='\.go''test|OZ_MIGRATED_APP_''RUN|migrated_app_''suite'
if matches="$(cd "$ROOT" && rg -n "$dynamic_runner_pattern" tests/specs tests/app internal/app 2>/dev/null || true)" && [[ -n "$matches" ]]; then
  printf '%s\n' "$matches" | tee -a "$LOG"
  fail "仍存在动态 gotest 或 migrated runner 引用"
fi
if gotests="$(cd "$ROOT" && fd -e gotest . tests internal/app 2>/dev/null || true)" && [[ -n "$gotests" ]]; then
  printf '%s\n' "$gotests" | tee -a "$LOG"
  fail "仓库可运行测试路径仍存在 gotest 文件"
fi

note "检查 tests/app 或 tests/specs 至少存在一个分组"
if [[ ! -d "$ROOT/tests/app" && ! -d "$ROOT/tests/specs" ]]; then
  fail "根目录 tests 缺少 app 或 specs 分组"
fi

note "检查核心真实测试包可单独运行"
(cd "$ROOT" && go test ./internal/app ./internal/ozcli ./cmd/oz ./tests -count=1) 2>&1 | tee -a "$LOG" || fail "核心真实测试包未通过"

note "检查 Go install 使用的包路径可构建"
tmp_bin_dir="$(mktemp -d)"
(cd "$ROOT" && go build -o "$tmp_bin_dir/oz" . && go build -o "$tmp_bin_dir/wo" ./cmd/oz) 2>&1 | tee -a "$LOG" || fail "根 oz 包或 cmd/oz 无法构建"

note "检查根目录 Go 测试门禁稳定"
(cd "$ROOT" && go test ./... -count=1) 2>&1 | tee -a "$LOG" || fail "go test ./... 未通过"

note "contract passed: migrated app tests no longer hide inside root gate"
