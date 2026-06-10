#!/usr/bin/env bash
# 文件功能目的：验证 QA 可以记录非阻断历史债务，但 acceptance_matrix 仍严格绑定 acceptance.json。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
result_dir="$repo_root/test-results/12-scope-gate"
test_file="$repo_root/internal/app/qa_acceptance_scope_contract_test.go"
log="$result_dir/qa-acceptance-scope.log"

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

note "写入 internal/app 包级合同测试，覆盖真实 QA artifact 和 acceptance matrix gate"
cat >"$test_file" <<'GO'
// Package app validates QA acceptance matrix scope boundaries.
package app

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCleanQAAllowsNonBlockingDebtOutsideAcceptanceMatrix records historical debt outside acceptance.
func TestCleanQAAllowsNonBlockingDebtOutsideAcceptanceMatrix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "qa-1.json")
	body := `{
  "summary": "当前 acceptance 合同已通过，历史债务仅记录为非阻断项",
  "decision": "clean",
  "findings": [],
  "non_blocking_findings": [
    {
      "title": "历史 CLI 输出缺少额外诊断",
      "severity": "major",
      "scope": "out_of_scope_existing",
      "evidence": "the missing diagnostic existed before this proposal and is not referenced by acceptance.json",
      "recommendation": "create a separate oz change for CLI diagnostics"
    }
  ],
  "acceptance_matrix": [
    {
      "id": "contract-demo",
      "status": "passed",
      "artifact": "docs/changes/12-收窄验收gate到提案范围/tests/test_qa_acceptance_scope_contract.sh",
      "evidence": "contract-demo command passed"
    },
    {
      "id": "runtime-demo",
      "status": "passed",
      "artifact": "test-results/12-scope-gate/qa-acceptance-scope.log",
      "evidence": "runtime log records QA scoped acceptance proof"
    }
  ],
  "evidence": [
    "go test ./internal/app -run TestCleanQAAllowsNonBlockingDebtOutsideAcceptanceMatrix passed",
    "runtime evidence: test-results/12-scope-gate/qa-acceptance-scope.log"
  ]
}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	qa, err := ReadQA(path)
	if err != nil {
		t.Fatalf("clean QA with non-blocking historical debt should validate, got %v", err)
	}
	if err := ValidateQAAgainstAcceptance(qa, scopedAcceptance()); err != nil {
		t.Fatalf("QA acceptance matrix should only need declared acceptance ids: %v", err)
	}
}

// TestCleanQAStillRejectsUnknownAcceptanceMatrixID prevents debt from expanding acceptance.
func TestCleanQAStillRejectsUnknownAcceptanceMatrixID(t *testing.T) {
	qa := QA{
		Summary:  "当前 acceptance 合同已通过但 matrix 混入历史债务 id",
		Decision: "clean",
		Findings: []Finding{},
		Evidence: []string{
			"go test ./internal/app -run TestCleanQAStillRejectsUnknownAcceptanceMatrixID passed",
			"runtime evidence: test-results/12-scope-gate/qa-acceptance-scope.log",
		},
		AcceptanceMatrix: []AcceptanceResult{
			{ID: "contract-demo", Status: "passed", Artifact: "docs/changes/12-收窄验收gate到提案范围/tests/test_qa_acceptance_scope_contract.sh", Evidence: "contract passed"},
			{ID: "runtime-demo", Status: "passed", Artifact: "test-results/12-scope-gate/qa-acceptance-scope.log", Evidence: "runtime log exists"},
			{ID: "legacy-debt-id", Status: "passed", Artifact: "test-results/12-scope-gate/debt.log", Evidence: "historical debt must not become acceptance"},
		},
	}

	if err := ValidateQAAgainstAcceptance(qa, scopedAcceptance()); err == nil {
		t.Fatal("acceptance_matrix must reject ids not declared by acceptance.json")
	}
}

// scopedAcceptance returns the current proposal's minimal acceptance contract.
func scopedAcceptance() Acceptance {
	return Acceptance{
		Summary: "scoped acceptance",
		RequiredTests: []AcceptanceTest{
			{ID: "contract-demo", Source: "change_contract", Path: "docs/changes/12-收窄验收gate到提案范围/tests/test_qa_acceptance_scope_contract.sh", Command: "bash docs/changes/12-收窄验收gate到提案范围/tests/test_qa_acceptance_scope_contract.sh", Purpose: "prove scoped QA acceptance", Assertions: []string{"QA acceptance_matrix covers only ids declared by acceptance.json"}},
		},
		RequiredEvidence: []AcceptanceEvidence{
			{ID: "runtime-demo", Kind: "runtime_log", Path: "test-results/12-scope-gate/qa-acceptance-scope.log", Purpose: "record scoped QA runtime evidence"},
		},
	}
}
GO

note "运行 Go 合同测试；当前实现预期失败于 QA non_blocking_findings 或 scope 字段缺失"
go test ./internal/app -run 'TestCleanQAAllowsNonBlockingDebtOutsideAcceptanceMatrix|TestCleanQAStillRejectsUnknownAcceptanceMatrixID' -count=1 2>&1 | tee -a "$log"

note "PASS"
