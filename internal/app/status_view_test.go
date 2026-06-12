// Package app tests compact workflow status rows for human status/watch output.
package app

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestStatusViewReadsImplementationContextDAGNodes verifies status/watch sees renamed execution context nodes.
func TestStatusViewReadsImplementationContextDAGNodes(t *testing.T) {
	state := statusViewImplementationContextState()
	state.DAGNodes = map[string]DAGNodeState{
		"implementation_context_1": {Status: statusRunning},
		"implementation_context_2": {Status: "success"},
	}

	view := buildStatusView(t.TempDir(), state, state.RunID, "")
	if marker := statusViewSubagentMarker(t, view, "代码库侦察员"); marker != "→" {
		t.Fatalf("running implementation_context marker = %q, want →", marker)
	}
	if marker := statusViewSubagentMarker(t, view, "外部资料研究员"); marker != "✓" {
		t.Fatalf("success implementation_context marker = %q, want ✓", marker)
	}
}

// TestStatusViewKeepsSkippedImplementationContextUnreached verifies completed-task skips do not look executed.
func TestStatusViewKeepsSkippedImplementationContextUnreached(t *testing.T) {
	state := statusViewImplementationContextState()

	view := buildStatusView(t.TempDir(), state, state.RunID, "")
	marker, found := statusViewOptionalSubagentMarker(view, "代码库侦察员")
	if found && marker != "-" {
		t.Fatalf("skipped implementation_context marker = %q, want -", marker)
	}
}

// TestCompactStatusLinesSkipsCompletedPlanningPlaceholder verifies pre-created proposals do not show a noise row.
func TestCompactStatusLinesSkipsCompletedPlanningPlaceholder(t *testing.T) {
	state := statusViewImplementationContextState()
	state.Stage = "execution"
	state.Stages = map[string]string{"planning": "completed", "execution": statusRunning}

	view := buildHumanStatusView(t.TempDir(), state, state.RunID, "")
	for _, line := range compactStatusLines(view) {
		if line == "规划阶段 - ✓ -" {
			t.Fatalf("planning placeholder should be hidden:\n%v", compactStatusLines(view))
		}
	}
}

// TestCompactStatusLinesSkipsEmptyPlanningPlaceholder verifies default sealed runs do not show idle planning.
func TestCompactStatusLinesSkipsEmptyPlanningPlaceholder(t *testing.T) {
	state := statusViewImplementationContextState()

	view := buildHumanStatusView(t.TempDir(), state, state.RunID, "")
	for _, line := range compactStatusLines(view) {
		if strings.Contains(line, "规划") {
			t.Fatalf("empty planning placeholder should be hidden:\n%v", compactStatusLines(view))
		}
	}
}

// TestHumanStatusMarksUnownedRunningRunStale verifies stale locks are not shown as live work.
func TestHumanStatusMarksUnownedRunningRunStale(t *testing.T) {
	repo := t.TempDir()
	state := statusViewImplementationContextState()
	state.RunID = "stale-status-run"
	if err := writeJSONFile(filepath.Join(runDir(repo, state.RunID), "lock"), LockInfo{PID: 99999999, RunID: state.RunID}); err != nil {
		t.Fatal(err)
	}

	lines := runProposalStatusLines(repo, state, "w1", "→")

	if lines[0] != "- demo x -" {
		t.Fatalf("header = %q, want stale marker", lines[0])
	}
	foundHint := false
	for _, line := range lines {
		if line == "  提示: 当前 run 的 lock 已失效，可运行 wo restart 重试当前阶段" {
			foundHint = true
		}
	}
	if !foundHint {
		t.Fatalf("missing stale restart hint in lines: %#v", lines)
	}
}

// statusViewImplementationContextState returns a minimal execution state with two configured helpers.
func statusViewImplementationContextState() State {
	workflow := DefaultWorkflowConfig()
	workflow.Engine = "go-dag"
	workflow.Parallel = ParallelConfig{
		Enabled: true,
		Groups: map[string]ParallelGroupConfig{
			"implementation_context": {
				Mode: "advisory",
				Members: []ParallelMemberConfig{
					{Name: "代码库侦察员", Purpose: "汇总 execution 需要读取的文件和测试模式", Tool: "pi"},
					{Name: "外部资料研究员", Purpose: "查询 execution 依赖的外部库文档", Tool: "pi"},
				},
			},
		},
	}
	return State{
		RunID:      "status-view-run",
		Status:     statusRunning,
		Stage:      "execution",
		Engine:     "go-dag",
		Sessions:   map[string]string{},
		Stages:     map[string]string{"execution": statusRunning},
		DAGNodes:   map[string]DAGNodeState{},
		Workflow:   workflow,
		ChangeName: "demo",
	}
}

// statusViewSubagentMarker finds one subagent row by full configured member name.
func statusViewSubagentMarker(t *testing.T, view statusView, fullName string) string {
	t.Helper()
	if marker, found := statusViewOptionalSubagentMarker(view, fullName); found {
		return marker
	}
	t.Fatalf("subagent row %q not found in %#v", fullName, view.Rows)
	return ""
}

// statusViewOptionalSubagentMarker returns a subagent marker only when the row is visible.
func statusViewOptionalSubagentMarker(view statusView, fullName string) (string, bool) {
	for _, row := range view.Rows {
		if row.Kind == "subagent" && row.FullName == fullName {
			return row.Marker, true
		}
	}
	return "", false
}
