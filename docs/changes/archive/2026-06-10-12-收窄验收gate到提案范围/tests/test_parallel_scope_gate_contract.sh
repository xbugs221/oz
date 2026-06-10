#!/usr/bin/env bash
# 文件功能目的：验证 parallel gate 只阻断当前提案范围内的 severe finding，并保持旧格式 finding 的硬阻断语义。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
result_dir="$repo_root/test-results/12-scope-gate"
test_file="$repo_root/internal/app/parallel_scope_gate_contract_test.go"
log="$result_dir/parallel-scope-gate.log"

mkdir -p "$result_dir"
: >"$log"

cleanup() {
  rm -f "$test_file"
}
trap cleanup EXIT

note() {
  # note 记录合同执行步骤，便于执行阶段判断失败是否来自目标行为缺失。
  printf '%s\n' "$*" | tee -a "$log"
}

cd "$repo_root"

note "写入 internal/app 包级合同测试，覆盖真实 ValidateParallelReviewGate"
cat >"$test_file" <<'GO'
// Package app validates scoped parallel gate behavior.
package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParallelReviewGateAllowsOutOfScopeExistingSevereFinding keeps historical debt non-blocking.
func TestParallelReviewGateAllowsOutOfScopeExistingSevereFinding(t *testing.T) {
	runPath := t.TempDir()
	writeParallelReviewArtifact(t, runPath, `{
      "title": "历史状态文件缺少旧字段迁移说明",
      "severity": "major",
      "scope": "out_of_scope_existing",
      "evidence": "legacy state files already lacked this explanation before the current diff",
      "recommendation": "track as a separate documentation debt change"
    }`)

	if err := ValidateParallelReviewGate(runPath, DefaultWorkflowConfig(), 1, cleanReviewForScopeGate()); err != nil {
		t.Fatalf("out-of-scope existing severe finding must not block clean review: %v", err)
	}
}

// TestParallelReviewGateBlocksCurrentChangeSevereFinding keeps current proposal defects strict.
func TestParallelReviewGateBlocksCurrentChangeSevereFinding(t *testing.T) {
	runPath := t.TempDir()
	writeParallelReviewArtifact(t, runPath, `{
      "title": "当前 diff 未满足 acceptance 合同",
      "severity": "major",
      "scope": "current_change",
      "evidence": "contract-demo fails after applying the current implementation",
      "recommendation": "fix the current implementation before review clean"
    }`)

	err := ValidateParallelReviewGate(runPath, DefaultWorkflowConfig(), 1, cleanReviewForScopeGate())
	if err == nil {
		t.Fatal("current-change severe finding must block clean review")
	}
}

// TestParallelReviewGateBlocksLegacyMissingScope preserves old artifact semantics.
func TestParallelReviewGateBlocksLegacyMissingScope(t *testing.T) {
	runPath := t.TempDir()
	writeParallelReviewArtifact(t, runPath, `{
      "title": "旧格式 major finding",
      "severity": "major",
      "evidence": "legacy parallel artifact has no scope field",
      "recommendation": "treat missing scope as current_change for backward compatibility"
    }`)

	err := ValidateParallelReviewGate(runPath, DefaultWorkflowConfig(), 1, cleanReviewForScopeGate())
	if err == nil {
		t.Fatal("legacy severe finding without scope must remain blocking")
	}
	if strings.Contains(strings.ToLower(err.Error()), "unknown field") {
		t.Fatalf("missing scope must be a valid legacy artifact, not a parse error: %v", err)
	}
}

// writeParallelReviewArtifact writes a complete configured parallel-review artifact.
func writeParallelReviewArtifact(t *testing.T, runPath string, findingJSON string) {
	t.Helper()
	body := `{
  "group": "review",
  "mode": "gate_input",
  "summary": "parallel reviewers completed scoped review",
  "members": [
    {"name":"目标核对审核员","purpose":"核对 proposal/spec/task 是否满足","status":"success","summary":"target checked","findings":[` + findingJSON + `]},
    {"name":"代码质量审核员","purpose":"检查类型、边界和可维护性","status":"success","summary":"quality checked"},
    {"name":"测试有效性审核员","purpose":"判断测试是否真实覆盖场景","status":"success","summary":"tests checked"},
    {"name":"安全风险审核员","purpose":"检查权限、输入、泄漏和破坏性操作","status":"success","summary":"security checked"},
    {"name":"上下文一致性审核员","purpose":"检查是否违背现有架构约定","status":"success","summary":"context checked"}
  ]
}`
	if err := os.WriteFile(filepath.Join(runPath, "parallel-review-1.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// cleanReviewForScopeGate returns a review artifact that is otherwise ready to pass.
func cleanReviewForScopeGate() Review {
	return Review{
		Summary:  "当前提案合同已满足",
		Decision: "clean",
		Checks: ReviewChecks{
			OzAligned:                true,
			TasksVerified:            true,
			TestsMeaningful:          true,
			ImplementationScoped:     true,
			RuntimeBehaviorVerified:  true,
			PreviousFindingsResolved: true,
		},
		Evidence: []string{
			"validation artifact passed: test-results/12-scope-gate/validation-review.json",
			"runtime evidence: QA trace test-results/12-scope-gate/parallel-scope.zip",
		},
		Findings: []Finding{},
	}
}
GO

note "运行 Go 合同测试；当前实现预期失败于 finding.scope 无法识别或 gate 未按 scope 判断"
go test ./internal/app -run 'TestParallelReviewGateAllowsOutOfScopeExistingSevereFinding|TestParallelReviewGateBlocksCurrentChangeSevereFinding|TestParallelReviewGateBlocksLegacyMissingScope' -count=1 2>&1 | tee -a "$log"

note "PASS"
