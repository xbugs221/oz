#!/usr/bin/env bash
# Sources: 12-收窄验收gate到提案范围
# 文件功能目的：验证 parallel review artifact 只作为主审核输入，原始子代理 finding 不能直接制造修复轮。
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

note "写入 internal/app 包级合同测试，覆盖真实 parallel review artifact 合同"
cat >"$test_file" <<'GO'
// Package app validates scoped parallel review artifact behavior.
package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParallelReviewArtifactAcceptsOutOfScopeExistingSevereFinding keeps historical debt as reviewer input.
func TestParallelReviewArtifactAcceptsOutOfScopeExistingSevereFinding(t *testing.T) {
	runPath := t.TempDir()
	path := writeParallelReviewArtifact(t, runPath, `{
      "title": "历史状态文件缺少旧字段迁移说明",
      "severity": "major",
      "scope": "out_of_scope_existing",
      "evidence": "legacy state files already lacked this explanation before the current diff",
      "recommendation": "track as a separate documentation debt change"
    }`)

	if _, err := ReadParallelArtifact(path); err != nil {
		t.Fatalf("out-of-scope existing severe finding must remain readable reviewer input: %v", err)
	}
}

// TestParallelReviewArtifactAcceptsCurrentChangeFindingAsInput keeps raw helper findings advisory.
func TestParallelReviewArtifactAcceptsCurrentChangeFindingAsInput(t *testing.T) {
	runPath := t.TempDir()
	path := writeParallelReviewArtifact(t, runPath, `{
      "title": "当前 diff 未满足 acceptance 合同",
      "severity": "major",
      "scope": "current_change",
      "evidence": "contract-demo fails after applying the current implementation",
      "recommendation": "fix the current implementation before review clean"
    }`)

	if _, err := ReadParallelArtifact(path); err != nil {
		t.Fatalf("raw parallel review finding must remain readable reviewer input: %v", err)
	}
}

// TestParallelReviewArtifactAcceptsLegacyMissingScopeAsReviewerInput preserves legacy parsing.
func TestParallelReviewArtifactAcceptsLegacyMissingScopeAsReviewerInput(t *testing.T) {
	runPath := t.TempDir()
	path := writeParallelReviewArtifact(t, runPath, `{
      "title": "旧格式 major finding",
      "severity": "major",
      "evidence": "legacy parallel artifact has no scope field",
      "recommendation": "treat missing scope as current_change for backward compatibility"
    }`)

	if _, err := ReadParallelArtifact(path); err != nil {
		t.Fatalf("legacy missing scope remains parse-compatible reviewer input: %v", err)
	}
}

// TestParallelArtifactRejectsNoActionBlockingFinding prevents positive confirmations from becoming blockers.
func TestParallelArtifactRejectsNoActionBlockingFinding(t *testing.T) {
	artifact := ParallelArtifact{
		Group:   "review",
		Mode:    "gate_input",
		Summary: "parallel reviewers completed scoped review",
		Members: []ParallelMemberResult{{
			Name:    "目标核对审核员",
			Status:  "success",
			Summary: "目标完整",
			Findings: []Finding{{
				Title:          "emass 四输出实现与 proposal 要求一致",
				Severity:       "blocker",
				Scope:          "current_change",
				Evidence:       "proposal/spec/task/acceptance 已经一一映射并通过",
				Recommendation: "满足。accepted.",
			}},
		}},
	}
	err := ValidateParallelArtifact(artifact)
	if err == nil {
		t.Fatal("no-action confirmation must not be accepted as a blocking finding")
	}
	if !strings.Contains(err.Error(), "无操作项") {
		t.Fatalf("error should explain the no-action blocker problem: %v", err)
	}
}

// writeParallelReviewArtifact writes a complete configured parallel-review artifact.
func writeParallelReviewArtifact(t *testing.T, runPath string, findingJSON string) string {
	t.Helper()
	path := filepath.Join(runPath, "parallel-review-1.json")
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
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
GO

note "运行 Go 合同测试；当前实现必须保留 parallel review 输入，并拒绝无操作 blocker"
go test ./internal/app -run 'TestParallelReviewArtifactAcceptsOutOfScopeExistingSevereFinding|TestParallelReviewArtifactAcceptsCurrentChangeFindingAsInput|TestParallelReviewArtifactAcceptsLegacyMissingScopeAsReviewerInput|TestParallelArtifactRejectsNoActionBlockingFinding' -count=1 2>&1 | tee -a "$log"

note "PASS"
