// Package app tests QA-specific parallel gate behavior.
package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAdvanceBlocksCleanQAWhenParallelNonRequiredMemberFails verifies gate_input failure is a hard QA gate.
func TestAdvanceBlocksCleanQAWhenParallelNonRequiredMemberFails(t *testing.T) {
	// Given: clean QA with all members present but one non-required gate_input member failed.
	repo := gitRepo(t)
	runID := "parallel-qa-non-required-failure-run"
	base := runDir(repo, runID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWritePrompt(t, filepath.Join(base, "acceptance.json"), acceptanceJSON())
	mustWritePrompt(t, filepath.Join(base, "qa-1.json"), cleanQAJSON())
	mustWritePrompt(t, filepath.Join(base, "parallel-qa-1.json"), parallelQAArtifactForTest(parallelMemberFixture{
		name:    "浏览器路径测试员",
		purpose: "execute browser path",
		status:  "failed",
		summary: "browser path failed without blocker finding",
	}))
	state := State{RunID: runID, ChangeName: "demo", Stage: "qa_1", Status: statusRunning, Workflow: parallelWorkflowForTest()}

	// When: the state machine advances the QA stage.
	err := (&Engine{Repo: repo}).advance(&state)

	// Then: archive remains blocked because gate_input member failure cannot be clean.
	if err == nil || !strings.Contains(err.Error(), "成员失败") {
		t.Fatalf("advance should reject non-required QA member failure, got %v", err)
	}
	if state.Stage != "qa_1" {
		t.Fatalf("stage advanced despite non-required QA member failure: %s", state.Stage)
	}
}
