#!/usr/bin/env bash
# 文件功能目的：验证 clean review 可以记录非阻断历史债务，同时 blocking findings 仍不能出现在 clean review。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
result_dir="$repo_root/test-results/12-scope-gate"
test_file="$repo_root/internal/app/review_non_blocking_debt_contract_test.go"
log="$result_dir/review-non-blocking-debt.log"

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

note "写入 internal/app 包级合同测试，覆盖真实 review artifact 解析与 gate 判断"
cat >"$test_file" <<'GO'
// Package app validates review scope contracts for non-blocking historical debt.
package app

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCleanReviewAcceptsNonBlockingHistoricalDebt verifies clean review can record out-of-scope debt.
func TestCleanReviewAcceptsNonBlockingHistoricalDebt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review-1.json")
	body := `{
  "summary": "当前提案合同已满足，历史债务仅记录为非阻断项",
  "decision": "clean",
  "findings": [],
  "non_blocking_findings": [
    {
      "title": "历史配置缺少更细粒度审计日志",
      "severity": "major",
      "scope": "out_of_scope_existing",
      "evidence": "baseline already lacks detailed audit logs before this change; current acceptance only covers scoped gate behavior",
      "recommendation": "create a separate oz change for audit-log hardening instead of blocking this sealed run"
    }
  ],
  "checks": {
    "oz_aligned": true,
    "tasks_verified": true,
    "tests_meaningful": true,
    "implementation_scoped": true,
    "runtime_behavior_verified": true,
    "previous_findings_resolved": true
  },
  "evidence": [
    "validation artifact passed: test-results/12-scope-gate/validation-review.json",
    "runtime evidence: QA trace test-results/12-scope-gate/review-scope.zip"
  ]
}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	review, err := ReadReview(path)
	if err != nil {
		t.Fatalf("clean review with non-blocking historical debt should validate, got %v", err)
	}
	if NeedsFix(review) {
		t.Fatalf("non-blocking historical debt must not trigger fix: %#v", review)
	}
}

// TestCleanReviewStillRejectsBlockingFindings proves the new debt channel does not weaken clean review.
func TestCleanReviewStillRejectsBlockingFindings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review-1.json")
	body := `{
  "summary": "当前提案仍有阻断问题",
  "decision": "clean",
  "findings": [
    {
      "title": "当前 diff 未实现 acceptance 合同",
      "severity": "major",
      "scope": "current_change",
      "evidence": "contract test contract-demo is still failing",
      "recommendation": "implement the acceptance behavior before clean review"
    }
  ],
  "checks": {
    "oz_aligned": true,
    "tasks_verified": true,
    "tests_meaningful": true,
    "implementation_scoped": true,
    "runtime_behavior_verified": true,
    "previous_findings_resolved": true
  },
  "evidence": [
    "validation artifact passed: test-results/12-scope-gate/validation-review.json",
    "runtime evidence: QA trace test-results/12-scope-gate/review-scope.zip"
  ]
}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ReadReview(path); err == nil {
		t.Fatal("clean review with blocking findings must still be rejected")
	}
}
GO

note "运行 Go 合同测试；当前实现预期失败于 non_blocking_findings 或 scope 字段缺失"
go test ./internal/app -run 'TestCleanReviewAcceptsNonBlockingHistoricalDebt|TestCleanReviewStillRejectsBlockingFindings' -count=1 2>&1 | tee -a "$log"

note "PASS"
