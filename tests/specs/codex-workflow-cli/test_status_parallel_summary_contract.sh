#!/usr/bin/env bash
# 文件功能目的：验证 wo status 人类输出能展示 parallel 并行成员摘要，并且 success 不能只由子会话记录推断。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/32-status-parallel/summary"
TEST_FILE="$ROOT/internal/app/status_parallel_summary_contract_test.go"

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

// TestHumanStatusShowsParallelMemberSummary 验证单 workflow status 在对应主阶段下展示并行组和成员摘要。
func TestHumanStatusShowsParallelMemberSummary(t *testing.T) {
	repo := gitRepo(t)
	workflow := statusParallelWorkflowForTest()
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
		},
		Workflow: workflow,
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	runPath := runDir(repo, state.RunID)
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
		"- 写 writer-session ✓",
		"  - 并行 implementation_context 2/2 success",
		"    - 代码库侦察员 success",
		"    - 外部资料研究员 success",
		"- 审 reviewer-session →",
		"  - 并行 review 1/2 failed",
		"    - 目标核对审核员 success",
		"    - 安全风险审核员 failed",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("status output missing %q:\n%s", want, got)
		}
	}
	for _, banned := range []string{"Sisyphus", "Prometheus", "Metis", "Momus", "Oracle", "Explore"} {
		if strings.Contains(got, banned) {
			t.Fatalf("status output leaked internal agent name %q:\n%s", banned, got)
		}
	}
}

// TestHumanStatusDoesNotTreatSubagentSessionAsSuccess 验证有子会话记录但缺失或非法 artifact 时不能显示 success。
func TestHumanStatusDoesNotTreatSubagentSessionAsSuccess(t *testing.T) {
	repo := gitRepo(t)
	workflow := statusParallelWorkflowForTest()
	state := State{
		RunID:      "parallel-status-missing",
		ChangeName: "demo",
		Sealed:     true,
		Status:     statusRunning,
		Stage:      "review_1",
		Sessions: map[string]string{
			"codex:reviewer": "reviewer-session",
			"codex:subagent:review:目标核对审核员:1": "subagent-session",
		},
		Workflow: workflow,
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}

	var missing bytes.Buffer
	if err := printHumanStatus(&missing, repo, "-w1"); err != nil {
		t.Fatal(err)
	}
	missingText := missing.String()
	statusSaveResult(t, "status-missing.txt", missingText)
	if !strings.Contains(missingText, "  - 并行 review 0/2 missing parallel-review-1.json") {
		t.Fatalf("missing artifact should be visible and non-success:\n%s", missingText)
	}
	if strings.Contains(missingText, "并行 review 2/2 success") {
		t.Fatalf("missing artifact was treated as success:\n%s", missingText)
	}

	artifactPath := filepath.Join(runDir(repo, state.RunID), "parallel-review-1.json")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifactPath, []byte("{not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	var invalid bytes.Buffer
	if err := printHumanStatus(&invalid, repo, "-w1"); err != nil {
		t.Fatal(err)
	}
	invalidText := invalid.String()
	statusSaveResult(t, "status-invalid.txt", invalidText)
	if !strings.Contains(invalidText, "  - 并行 review 0/2 invalid parallel-review-1.json") {
		t.Fatalf("invalid artifact should be visible and non-success:\n%s", invalidText)
	}
	if strings.Contains(invalidText, "并行 review 2/2 success") {
		t.Fatalf("invalid artifact was treated as success:\n%s", invalidText)
	}
}

// TestHumanStatusRejectsUnconfiguredParallelMembers 验证 artifact 成员集合必须等于配置成员集合，不能展示或统计未配置成员。
func TestHumanStatusRejectsUnconfiguredParallelMembers(t *testing.T) {
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
			state := State{
				RunID:      "parallel-status-unconfigured-" + tc.name,
				ChangeName: "demo",
				Sealed:     true,
				Status:     statusRunning,
				Stage:      "review_1",
				Sessions:   map[string]string{"codex:reviewer": "reviewer-session"},
				Workflow:   workflow,
			}
			if err := saveState(repo, state); err != nil {
				t.Fatal(err)
			}
			statusWriteParallelArtifact(t, filepath.Join(runDir(repo, state.RunID), "parallel-review-1.json"), ParallelArtifact{
				Group:   "review",
				Mode:    "gate_input",
				Summary: "review helpers completed",
				Members: []ParallelMemberResult{
					{Name: "目标核对审核员", Purpose: "核对目标", Status: "success", Summary: "target matches"},
					{Name: "安全风险审核员", Purpose: "检查风险", Status: "success", Summary: "no risk found"},
					{Name: "未配置审核员", Purpose: "should not be visible", Status: tc.status, Summary: "poisoned member"},
				},
			})

			var stdout bytes.Buffer
			if err := printHumanStatus(&stdout, repo, "-w1"); err != nil {
				t.Fatal(err)
			}
			got := stdout.String()
			statusSaveResult(t, "status-unconfigured-"+tc.name+".txt", got)
			if !strings.Contains(got, "  - 并行 review 0/2 invalid parallel-review-1.json") {
				t.Fatalf("unconfigured member should make artifact invalid:\n%s", got)
			}
			for _, banned := range []string{"未配置审核员", "并行 review 2/2 success", "并行 review 3/2 failed"} {
				if strings.Contains(got, banned) {
					t.Fatalf("status output should not trust unconfigured member %q:\n%s", banned, got)
				}
			}
		})
	}
}

// TestBatchHumanStatusShowsParallelSummaryUnderChange 验证 batch status 保持 change 层级并在对应 run 下展示并行摘要。
func TestBatchHumanStatusShowsParallelSummaryUnderChange(t *testing.T) {
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
		"  - 写 writer-session →",
		"    - 并行 implementation_context 2/2 success",
		"      - 代码库侦察员 success",
		"      - 外部资料研究员 success",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("batch status output missing %q:\n%s", want, got)
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

WO_STATUS_PARALLEL_RESULT_DIR="$RESULT_DIR" go test ./internal/app -run 'TestHumanStatusShowsParallelMemberSummary|TestHumanStatusDoesNotTreatSubagentSessionAsSuccess|TestHumanStatusRejectsUnconfiguredParallelMembers|TestBatchHumanStatusShowsParallelSummaryUnderChange' -count=1 -v | tee "$RESULT_DIR/contract.log"
