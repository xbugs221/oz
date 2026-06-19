// Package app tests read-only subagent runtime artifact boundaries.
package app

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestSubagentBoundaryRestoresWrongSiblingMemberArtifact verifies advisory guard repairs polluted helper output.
func TestSubagentBoundaryRestoresWrongSiblingMemberArtifact(t *testing.T) {
	repo := gitRepo(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	runID := "20260618T142126.063796629Z"
	changeName := "29-demo"
	testMember := ParallelMemberConfig{Name: "测试有效性审核员", Purpose: "判断测试是否真实覆盖场景", Tool: "pi", Required: true}
	contextMember := ParallelMemberConfig{Name: "上下文一致性审核员", Purpose: "检查是否违背现有架构约定", Tool: "pi", Required: true}
	workflow := WorkflowConfig{
		Parallel: ParallelConfig{
			Enabled: true,
			Groups: map[string]ParallelGroupConfig{
				"review": {
					Mode:    "gate_input",
					Members: []ParallelMemberConfig{testMember, contextMember},
				},
			},
		},
		SubagentGuard: subagentGuardModeAdvisory,
	}
	state := State{RunID: runID, ChangeName: changeName, Workflow: workflow}
	testPath := memberArtifactPath(repo, runID, "review", 1, testMember.Name)
	contextPath := memberArtifactPath(repo, runID, "review", 1, contextMember.Name)

	if err := writeMemberArtifact(testPath, ParallelMemberResult{
		Name:       testMember.Name,
		ChangeName: changeName,
		Purpose:    testMember.Purpose,
		Status:     "success",
		Summary:    "tests are meaningful",
		Evidence:   []string{"contract tests cover real workflow"},
	}); err != nil {
		t.Fatal(err)
	}
	beforeRunFiles, err := runArtifactFileSnapshot(runDir(repo, runID))
	if err != nil {
		t.Fatal(err)
	}
	beforeHead, beforeDiff, err := gitSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	beforeContent, err := gitChangeContentSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}

	if err := writeMemberArtifact(testPath, ParallelMemberResult{
		Name:       contextMember.Name,
		ChangeName: changeName,
		Purpose:    contextMember.Purpose,
		Status:     "success",
		Summary:    "architecture is consistent",
		Evidence:   []string{"module boundaries inspected"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := writeMemberArtifact(contextPath, ParallelMemberResult{
		Name:       contextMember.Name,
		ChangeName: changeName,
		Purpose:    contextMember.Purpose,
		Status:     "success",
		Summary:    "architecture is consistent",
		Evidence:   []string{"module boundaries inspected"},
	}); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine(repo, nil)
	repair, err := engine.checkSubagentReadOnlyBoundary(state, contextMember, 1, contextPath, subagentGuardModeAdvisory, beforeHead, beforeDiff, beforeContent, beforeRunFiles)
	if err != nil {
		t.Fatal(err)
	}
	revertedPath, err := filepath.Rel(runDir(repo, runID), testPath)
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(repair.Reverted, filepath.ToSlash(revertedPath)) {
		t.Fatalf("reverted paths = %#v, want %s", repair.Reverted, filepath.ToSlash(revertedPath))
	}
	if len(repair.Advisory) == 0 || !strings.Contains(repair.Advisory[0], "run artifact changed outside ARTIFACT_DIR") {
		t.Fatalf("advisory = %#v, want run artifact boundary note", repair.Advisory)
	}
	result, err := readMemberArtifact(testPath)
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != testMember.Name || result.Summary != "tests are meaningful" {
		t.Fatalf("restored member = %#v, want original test member artifact", result)
	}
}

// containsString reports whether values contains target exactly.
func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
