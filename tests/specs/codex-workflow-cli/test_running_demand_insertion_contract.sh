#!/usr/bin/env bash
# 文件功能目的：验证 sealed run 期间允许新增非当前需求，同时继续阻断当前 run 相关路径变化。
# Sources: 16-允许运行中追加新需求但保留subagent写保护
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
LOG="$ROOT/test-results/running-demand-insertion.log"

note() {
  # 函数目的：记录规格测试步骤和 go test 输出。
  printf '[running-demand] %s\n' "$*" | tee -a "$LOG"
}

mkdir -p "$(dirname "$LOG")"
: >"$LOG"

note "运行长期 Go 回归，验证非当前 change 可追加且当前 run/source/rename 仍受保护"
(
  cd "$ROOT"
  OZ_MIGRATED_APP_RUN='TestDetectManualInterventionAllowsUnrelatedActiveChange|TestDetectManualInterventionIgnoresExistingProtectedBaselineDiff|TestDetectManualInterventionBlocksCurrentRunPaths|TestDetectManualInterventionBlocksCurrentChangeRename' \
    go test ./tests/app -run TestMigratedAppTestsRunWithGoToolchain -count=1
) 2>&1 | tee -a "$LOG"

note "contract passed: running demand insertion keeps current-run guards"
