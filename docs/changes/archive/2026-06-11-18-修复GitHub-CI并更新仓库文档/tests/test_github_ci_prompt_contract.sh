#!/usr/bin/env bash
# 文件功能目的：复现并约束 GitHub CI 中 planning prompt 合同失败，确保修复后本地等价 Go 测试通过。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/18-github-ci-docs"
LOG="$RESULT_DIR/github-ci-prompt-contract.log"
RUN_SUMMARY="$RESULT_DIR/github-run-27288329734.json"

mkdir -p "$RESULT_DIR"
: >"$LOG"

note() {
  # 函数目的：同时写入终端和 runtime log，方便执行阶段定位失败步骤。
  printf '[github-ci-prompt] %s\n' "$*" | tee -a "$LOG"
}

fail() {
  # 函数目的：用业务可读错误终止契约测试，避免把 CI 失败误判为环境问题。
  printf '[github-ci-prompt] FAIL: %s\n' "$*" | tee -a "$LOG" >&2
  exit 1
}

write_known_run_summary() {
  # 函数目的：记录创建阶段已确认的 GitHub run 摘要；执行阶段可用 gh 输出覆盖该文件。
  cat >"$RUN_SUMMARY" <<'JSON'
{
  "run_id": 27288329734,
  "url": "https://github.com/xbugs221/oz/actions/runs/27288329734",
  "workflow": "CI",
  "head_branch": "main",
  "head_sha": "7fd5b2780da48c384e89d5987aa67620f01939fc",
  "display_title": "subagent defaults to pi",
  "failed_job": "Test",
  "failed_step": "Run Go tests",
  "local_reproduction": [
    "TestParallelEnabledPromptsCarryFanoutArtifacts/planning missing 讨论规划阶段",
    "TestBundledOzSkillPromptsDelegateToSkills missing 讨论规划阶段"
  ]
}
JSON
}

run_prompt_go_contract() {
  # 函数目的：兼容迁移前后的测试布局，运行真实 app prompt 合同而不是静态 grep 代替 Go 测试。
  if [[ -f "$ROOT/tests/app/migrated_app_suite_test.go" ]]; then
    note "运行迁移后的 app prompt 合同测试"
    (
      cd "$ROOT"
      OZ_MIGRATED_APP_RUN='TestParallelEnabledPromptsCarryFanoutArtifacts|TestBundledOzSkillPromptsDelegateToSkills' \
        go test ./tests/app -run TestMigratedAppTestsRunWithGoToolchain -count=1
    ) 2>&1 | tee -a "$LOG"
    return "${PIPESTATUS[0]}"
  fi

  note "运行旧 internal/app prompt 合同测试"
  (
    cd "$ROOT"
    go test ./internal/app -run 'TestParallelEnabledPromptsCarryFanoutArtifacts|TestBundledOzSkillPromptsDelegateToSkills' -count=1
  ) 2>&1 | tee -a "$LOG"
  return "${PIPESTATUS[0]}"
}

write_known_run_summary
note "已写入 GitHub run 摘要：$RUN_SUMMARY"

if ! run_prompt_go_contract; then
  fail "planning prompt Go 合同仍未通过；需要修复 GitHub CI 中缺失的“讨论规划阶段”语义"
fi

PROMPT="$ROOT/prompts-template/wo-discuss.md"
[[ -f "$PROMPT" ]] || fail "缺少 bundled planning prompt: $PROMPT"

note "检查 wo-discuss 同时保留 skill 入口和阶段语义"
grep -q 'oz-plan' "$PROMPT" || fail "wo-discuss 必须包含 oz-plan 技能入口"
grep -q '讨论规划阶段' "$PROMPT" || fail "wo-discuss 必须包含“讨论规划阶段”语义"

note "contract passed: GitHub CI planning prompt 合同已恢复"
