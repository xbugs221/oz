// Package app contains long-lived regression tests migrated from shell-injected contracts.
package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGoDAGHumanStatusContract verifies the user-facing status summary for the default Go DAG engine.
func TestGoDAGHumanStatusContract(t *testing.T) {
	repo := gitRepo(t)
	workflow := DefaultWorkflowConfig()
	workflow.Parallel.Enabled = true
	state := State{
		RunID:      "go-dag-status-run",
		ChangeName: "demo",
		Sealed:     true,
		Status:     statusRunning,
		Stage:      "review_1",
		Sessions:   map[string]string{"codex:executor": "exec-session"},
		Stages:     map[string]string{"execution": "completed"},
		Workflow:   workflow,
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	base := runDir(repo, state.RunID)
	mustWritePrompt(t, filepath.Join(base, "parallel-planning-context.json"), parallelPlanningArtifactForTest())
	mustWritePrompt(t, filepath.Join(base, "parallel-implementation-context.json"), parallelContextArtifactForTest())
	mustWritePrompt(t, filepath.Join(base, "parallel-review-1.json"), parallelReviewArtifactForTest())
	var stdout bytes.Buffer
	if err := printHumanStatus(&stdout, repo, "-w1"); err != nil {
		t.Fatal(err)
	}
	text := stdout.String()
	for _, want := range []string{
		"- demo → -",
		"  执行 exec-session ✓ -",
		"  审核 -            → -",
		"  修正 -            - -",
		"  测试 -            - -",
		"  归档 -            - -",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("status output missing %q:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{"引擎 go-dag", "- 并行", "planning_context", "implementation_context", "parallel-review"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("status output leaked internal status text %q:\n%s", forbidden, text)
		}
	}
	_ = os.RemoveAll(repo)
}

// mustWritePrompt writes JSON-like test artifacts for status rendering fixtures.
func mustWritePrompt(t *testing.T, path string, value any) {
	t.Helper()
	if err := writeJSONFile(path, value); err != nil {
		t.Fatal(err)
	}
}

// parallelPlanningArtifactForTest returns a compact planning fan-in artifact.
func parallelPlanningArtifactForTest() ParallelArtifact {
	return ParallelArtifact{
		Group:   "planning_context",
		Mode:    "advisory",
		Summary: "planning completed",
		Members: []ParallelMemberResult{{Name: "需求分析员", Purpose: "分析需求", Status: "success", Summary: "ok"}},
	}
}

// parallelContextArtifactForTest returns a compact implementation fan-in artifact.
func parallelContextArtifactForTest() ParallelArtifact {
	return ParallelArtifact{
		Group:   "implementation_context",
		Mode:    "advisory",
		Summary: "implementation completed",
		Members: []ParallelMemberResult{
			{Name: "代码库侦察员", Purpose: "搜索现有模块", Status: "success", Summary: "ok"},
			{Name: "外部资料研究员", Purpose: "查询外部资料", Status: "success", Summary: "ok"},
		},
	}
}

// parallelReviewArtifactForTest returns a compact review fan-in artifact.
func parallelReviewArtifactForTest() ParallelArtifact {
	return ParallelArtifact{
		Group:   "review",
		Mode:    "gate_input",
		Summary: "review completed",
		Members: []ParallelMemberResult{{Name: "目标核对审核员", Purpose: "核对目标", Status: "success", Summary: "ok"}},
	}
}
