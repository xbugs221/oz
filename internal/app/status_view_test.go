// Package app tests compact workflow status rows for human status/watch output.
package app

import (
	"bytes"
	"encoding/json"
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

// TestStatusViewRendersRunningBatch verifies batch status shows progress, active run rows, and stale restart guidance.
func TestStatusViewRendersRunningBatch(t *testing.T) {
	repo := t.TempDir()
	state := statusViewImplementationContextState()
	state.RunID = "batch-running-run"
	state.ChangeName = "23-统一状态展示视图模型"
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(filepath.Join(runDir(repo, state.RunID), "lock"), LockInfo{PID: 99999999, RunID: state.RunID}); err != nil {
		t.Fatal(err)
	}
	batch := BatchState{
		BatchID:      "batch-running",
		Status:       batchStatusRunning,
		Changes:      []string{"01-已完成", state.ChangeName},
		CurrentIndex: 1,
		RunIDs:       map[string]string{state.ChangeName: state.RunID},
	}

	lines := batchStatusLines(repo, &batch, "b1", nil)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "批量任务 b1 running 2/2") {
		t.Fatalf("running batch output missing progress:\n%s", joined)
	}
	if !strings.Contains(joined, "- "+state.ChangeName+" x -") || !strings.Contains(joined, "执行") {
		t.Fatalf("running batch output missing current stale run rows:\n%s", joined)
	}
	if !strings.Contains(joined, "wo restart -b1 重试当前批量阶段") {
		t.Fatalf("running batch output missing stale batch restart hint:\n%s", joined)
	}
}

// TestStatusViewRendersFailedBatch verifies stopped batches expose failure reason and restart guidance.
func TestStatusViewRendersFailedBatch(t *testing.T) {
	repo := t.TempDir()
	state := statusViewImplementationContextState()
	state.RunID = "batch-failed-run"
	state.ChangeName = "23-统一状态展示视图模型"
	state.Status = statusFailed
	state.Stage = "execution"
	state.Error = "测试失败"
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	batch := BatchState{
		BatchID:      "batch-failed",
		Status:       batchStatusFailed,
		Changes:      []string{"01-已完成", state.ChangeName},
		CurrentIndex: 1,
		RunIDs:       map[string]string{state.ChangeName: state.RunID},
		FailedChange: state.ChangeName,
		FailedRunID:  state.RunID,
	}

	lines := batchStatusLines(repo, &batch, "b2", nil)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "- "+state.ChangeName+" x") {
		t.Fatalf("failed batch output missing failed run row:\n%s", joined)
	}
	if !strings.Contains(joined, "错误: "+state.ChangeName+" 的写阶段失败：测试失败") {
		t.Fatalf("failed batch output missing failure summary:\n%s", joined)
	}
	if !strings.Contains(joined, "wo restart -b2 删除失败记录并继续该批量任务") {
		t.Fatalf("failed batch output missing restart guidance:\n%s", joined)
	}
}

// TestPrintHumanStatusRendersSharedView verifies the status command reads durable state and renders compact rows.
func TestPrintHumanStatusRendersSharedView(t *testing.T) {
	repo := t.TempDir()
	state := statusViewImplementationContextState()
	state.RunID = "status-print-run"
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := printHumanStatus(&stdout, repo, "-w1"); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	if !strings.Contains(out, "- demo →") {
		t.Fatalf("status output missing shared header:\n%s", out)
	}
	if !strings.Contains(out, "执行") || !strings.Contains(out, "→") {
		t.Fatalf("status output missing compact execution row:\n%s", out)
	}
}

// TestWatchRendersSharedViewWithSpinner verifies watch output swaps the running marker through the renderer.
func TestWatchRendersSharedViewWithSpinner(t *testing.T) {
	repo := t.TempDir()
	state := statusViewImplementationContextState()
	state.RunID = "status-watch-run"
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}

	lines := watchStatusLines(repo, "run", StatusRef{Alias: "w1", ID: state.RunID}, "|")
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "- demo |") {
		t.Fatalf("watch output missing spinner header:\n%s", joined)
	}
	if !strings.Contains(joined, "执行") || !strings.Contains(joined, "|") {
		t.Fatalf("watch output missing spinner compact row:\n%s", joined)
	}
}

// TestStatusHeaderUsesWorkflowWallTime verifies the header measures elapsed run time, not row totals.
func TestStatusHeaderUsesWorkflowWallTime(t *testing.T) {
	state := statusViewImplementationContextState()
	state.RunID = "20260525T000000.000000000Z"
	state.Status = statusDone
	state.Stage = statusDone
	state.Stages = map[string]string{
		"execution": "completed",
		"review_1":  "completed",
	}
	state.StageTimings = map[string]StageTiming{
		"execution": {StartedAt: "2026-05-25T00:10:00Z", FinishedAt: "2026-05-25T00:20:00Z"},
		"review_1":  {StartedAt: "2026-05-25T01:00:00Z", FinishedAt: "2026-05-25T01:30:00Z"},
	}
	state.DAGNodes = map[string]DAGNodeState{
		"implementation_context_1": {
			Status:     "success",
			StartedAt:  "2026-05-25T00:05:00Z",
			FinishedAt: "2026-05-25T00:25:00Z",
		},
	}

	view := buildHumanStatusView(t.TempDir(), state, state.RunID, "")
	header := statusHeaderText(state.ChangeName, view)

	if !strings.Contains(header, "90.00 分钟") {
		t.Fatalf("header = %q, want wall time from run start to last activity", header)
	}
	if strings.Contains(header, "60.00 分钟") {
		t.Fatalf("header = %q, should not sum visible stage and subagent durations", header)
	}
}

// TestRunnerStatusViewSerializesObservability verifies runner JSON keeps the shared status view contract.
func TestRunnerStatusViewSerializesObservability(t *testing.T) {
	repo := t.TempDir()
	state := statusViewImplementationContextState()
	dto := runnerStateFromStatusView(repo, state, "w1")
	data, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Observability struct {
			DisplayID string `json:"display_id"`
			Engine    string `json:"engine"`
			Rows      []struct {
				Kind   string `json:"kind"`
				Name   string `json:"name"`
				Marker string `json:"marker"`
			} `json:"rows"`
		} `json:"observability"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Observability.DisplayID != "w1" || payload.Observability.Engine != "go-dag" {
		t.Fatalf("unexpected observability identity: %#v", payload.Observability)
	}
	for _, row := range payload.Observability.Rows {
		if row.Kind == "stage" && row.Name == "执行阶段" && row.Marker == "→" {
			return
		}
	}
	t.Fatalf("runner observability missing running execution row: %s", data)
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
