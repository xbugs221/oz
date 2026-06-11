#!/usr/bin/env bash
# 文件功能目的：验证 wo status 人类输出不泄漏 parallel fan-in 摘要，同时机器 gate 仍严格校验 parallel artifact。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/32-status-parallel/summary"
TEST_FILE="$ROOT/tests/app/status_parallel_summary_contract_test.gotest"

rm -rf "$RESULT_DIR"
mkdir -p "$RESULT_DIR"
cleanup() {
  rm -f "$TEST_FILE"
}
trap cleanup EXIT

cd "$ROOT"

cat > "$TEST_FILE" <<'GO'
package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHumanStatusHidesParallelFanInSummary 验证单 workflow status 只展示极简阶段和 helper 行，不展示 fan-in 摘要。
func TestHumanStatusHidesParallelFanInSummary(t *testing.T) {
	repo := gitRepo(t)
	workflow := statusParallelWorkflowForTest()
	runPath := runDir(repo, "parallel-status-single")
	targetArtifact := filepath.Join(runPath, "parallel-members", "review", "1", "target.json")
	riskArtifact := filepath.Join(runPath, "parallel-members", "review", "1", "risk.json")
	state := State{
		RunID:      "parallel-status-single",
		ChangeName: "demo",
		Sealed:     true,
		Status:     statusRunning,
		Stage:      "review_1",
		Stages:     map[string]string{"execution": "completed"},
		Sessions: map[string]string{
			"codex:executor": "writer-session",
			"codex:reviewer": "reviewer-session",
			"pi:subagent:review:目标核对审核员:1": "target-session",
			"pi:subagent:review:安全风险审核员:1": "risk-session",
		},
		DAGNodes: map[string]DAGNodeState{
			"before_review_1_1": {Status: "success", Artifact: targetArtifact},
			"before_review_1_2": {Status: "failed", Artifact: riskArtifact},
		},
		Workflow: workflow,
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(targetArtifact, ParallelMemberResult{Name: "目标核对审核员", Purpose: "核对目标", Status: "success", Summary: "target matches"}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(riskArtifact, ParallelMemberResult{Name: "安全风险审核员", Purpose: "检查风险", Status: "failed", Summary: "found a risk"}); err != nil {
		t.Fatal(err)
	}
	statusWriteParallelArtifact(t, filepath.Join(runPath, "parallel-implementation-context.json"), ParallelArtifact{
		Group:   "implementation_context",
		Mode:    "advisory",
		Summary: "implementation context completed",
		Members: []ParallelMemberResult{
			{Name: "代码库侦察员", Purpose: "梳理既有代码模式", Status: "success", Summary: "found local patterns"},
			{Name: "外部资料研究员", Purpose: "核对外部资料", Status: "success", Summary: "checked docs"},
		},
	})
	statusWriteParallelArtifact(t, filepath.Join(runPath, "parallel-review-1.json"), ParallelArtifact{
		Group:   "review",
		Mode:    "gate_input",
		Summary: "review helpers completed",
		Members: []ParallelMemberResult{
			{Name: "目标核对审核员", Purpose: "核对目标", Status: "success", Summary: "target matches"},
			{Name: "安全风险审核员", Purpose: "检查风险", Status: "failed", Summary: "found a risk"},
		},
	})

	var stdout bytes.Buffer
	if err := printHumanStatus(&stdout, repo, "-w1"); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	statusSaveResult(t, "status-single.txt", got)
	for _, want := range []string{
		"  执行阶段 writer-session ✓",
		"  审核阶段 reviewer-session →",
		"    目标核对 target-session ✓",
		"    风险检查 risk-session x",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("status output missing %q:\n%s", want, got)
		}
	}
	for _, banned := range []string{"- 并行", "implementation_context", "parallel-review", "代码库侦察员 success", "安全风险审核员 failed", "Sisyphus", "Prometheus", "Metis", "Momus", "Oracle", "Explore"} {
		if strings.Contains(got, banned) {
			t.Fatalf("status output leaked internal agent name %q:\n%s", banned, got)
		}
	}
}

// TestParallelReviewGateRejectsMissingOrInvalidArtifact 验证缺失或非法 fan-in artifact 仍通过机器 gate 暴露为非成功。
func TestParallelReviewGateRejectsMissingOrInvalidArtifact(t *testing.T) {
	repo := gitRepo(t)
	workflow := statusParallelWorkflowForTest()
	runPath := runDir(repo, "parallel-status-missing")
	if err := ValidateParallelReviewGate(runPath, workflow, 1, cleanReviewForParallelStatusTest()); err == nil || !strings.Contains(err.Error(), "parallel-review-1.json") {
		t.Fatalf("missing artifact should block clean review with path, got %v", err)
	}

	artifactPath := filepath.Join(runPath, "parallel-review-1.json")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifactPath, []byte("{not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateParallelReviewGate(runPath, workflow, 1, cleanReviewForParallelStatusTest()); err == nil || !strings.Contains(err.Error(), "parseError") {
		t.Fatalf("invalid artifact should block clean review as parse error, got %v", err)
	}
}

// TestParallelReviewGateRejectsUnconfiguredParallelMembers 验证 artifact 成员集合必须等于配置成员集合，不能信任未配置成员。
func TestParallelReviewGateRejectsUnconfiguredParallelMembers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status string
	}{
		{name: "extra-success", status: "success"},
		{name: "extra-failed", status: "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := gitRepo(t)
			workflow := statusParallelWorkflowForTest()
			runPath := runDir(repo, "parallel-status-unconfigured-"+tc.name)
			statusWriteParallelArtifact(t, filepath.Join(runPath, "parallel-review-1.json"), ParallelArtifact{
				Group:   "review",
				Mode:    "gate_input",
				Summary: "review helpers completed",
				Members: []ParallelMemberResult{
					{Name: "目标核对审核员", Purpose: "核对目标", Status: "success", Summary: "target matches"},
					{Name: "安全风险审核员", Purpose: "检查风险", Status: "success", Summary: "no risk found"},
					{Name: "未配置审核员", Purpose: "should not be visible", Status: tc.status, Summary: "poisoned member"},
				},
			})
			if err := ValidateParallelReviewGate(runPath, workflow, 1, cleanReviewForParallelStatusTest()); err == nil || !strings.Contains(err.Error(), "未配置成员") {
				t.Fatalf("unconfigured member should make artifact invalid, got %v", err)
			}
		})
	}
}

// TestBatchHumanStatusHidesParallelSummaryUnderChange 验证 batch status 保持 change 层级且不展示 fan-in 摘要。
func TestBatchHumanStatusHidesParallelSummaryUnderChange(t *testing.T) {
	repo := gitRepo(t)
	workflow := statusParallelWorkflowForTest()
	state := State{
		RunID:      "parallel-status-batch-run",
		ChangeName: "1-demo",
		Sealed:     true,
		Status:     statusRunning,
		Stage:      "execution",
		BatchID:    "parallel-status-batch",
		Sessions:   map[string]string{"codex:executor": "writer-session"},
		Workflow:   workflow,
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	statusWriteParallelArtifact(t, filepath.Join(runDir(repo, state.RunID), "parallel-implementation-context.json"), ParallelArtifact{
		Group:   "implementation_context",
		Mode:    "advisory",
		Summary: "implementation context completed",
		Members: []ParallelMemberResult{
			{Name: "代码库侦察员", Purpose: "梳理既有代码模式", Status: "success", Summary: "found local patterns"},
			{Name: "外部资料研究员", Purpose: "核对外部资料", Status: "success", Summary: "checked docs"},
		},
	})
	batch := BatchState{
		BatchID:      "parallel-status-batch",
		Status:       batchStatusRunning,
		Changes:      []string{"1-demo"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{"1-demo": state.RunID},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := printHumanStatus(&stdout, repo); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	statusSaveResult(t, "status-batch.txt", got)
	for _, want := range []string{
		"批量任务 b1 running 1/1",
		"- 1-demo",
		"  执行阶段 writer-session →",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("batch status output missing %q:\n%s", want, got)
		}
	}
	for _, banned := range []string{"- 并行", "implementation_context", "代码库侦察员 success", "外部资料研究员 success"} {
		if strings.Contains(got, banned) {
			t.Fatalf("batch status output leaked fan-in %q:\n%s", banned, got)
		}
	}
}

func statusParallelWorkflowForTest() WorkflowConfig {
	workflow := DefaultWorkflowConfig()
	workflow.Parallel.Enabled = true
	workflow.Parallel.Groups["implementation_context"] = ParallelGroupConfig{
		Mode: "advisory",
		Members: []ParallelMemberConfig{
			{Name: "代码库侦察员", Purpose: "梳理既有代码模式"},
			{Name: "外部资料研究员", Purpose: "核对外部资料"},
		},
	}
	workflow.Parallel.Groups["review"] = ParallelGroupConfig{
		Mode: "gate_input",
		Members: []ParallelMemberConfig{
			{Name: "目标核对审核员", Purpose: "核对目标"},
			{Name: "安全风险审核员", Purpose: "检查风险"},
		},
	}
	return workflow
}

func statusWriteParallelArtifact(t *testing.T, path string, artifact ParallelArtifact) {
	t.Helper()
	if err := writeJSONFile(path, artifact); err != nil {
		t.Fatal(err)
	}
}

// cleanReviewForParallelStatusTest 构造 clean review，专门触发 parallel gate 对 fan-in artifact 的校验。
func cleanReviewForParallelStatusTest() Review {
	return Review{
		Summary:  "review clean",
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
			"validation artifact passed: validation-review-1.json",
			"runtime evidence: QA trace test-results/status-parallel.json",
		},
	}
}

func statusSaveResult(t *testing.T, name string, text string) {
	t.Helper()
	resultDir := os.Getenv("WO_STATUS_PARALLEL_RESULT_DIR")
	if resultDir == "" {
		return
	}
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resultDir, name), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
GO

WO_STATUS_PARALLEL_RESULT_DIR="$RESULT_DIR" \
  OZ_MIGRATED_APP_RUN='TestHumanStatusHidesParallelFanInSummary|TestParallelReviewGateRejectsMissingOrInvalidArtifact|TestParallelReviewGateRejectsUnconfiguredParallelMembers|TestBatchHumanStatusHidesParallelSummaryUnderChange' \
  go test ./tests/app -run TestMigratedAppTestsRunWithGoToolchain -count=1 -v | tee "$RESULT_DIR/contract.log"
