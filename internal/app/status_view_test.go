// Package app tests compact workflow status rows for human status/watch output.
package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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

// TestCompactStatusLinesAlignsLongDurationColumn verifies multi-digit minutes keep a stable duration column.
func TestCompactStatusLinesAlignsLongDurationColumn(t *testing.T) {
	shortMinutes := 9.5
	longMinutes := 1234.56
	view := statusView{
		Rows: []statusViewRow{
			{Name: "执行阶段", Kind: "stage", SessionID: "s1", Marker: "✓", DurationMinutes: &longMinutes},
			{Name: "审核阶段", Kind: "stage", SessionID: "s2", Marker: "✓", DurationMinutes: &shortMinutes},
			{Name: "测试阶段", Kind: "stage", SessionID: "s3", Marker: "✓", DurationMinutes: nil},
		},
	}

	lines := compactStatusLines(view)
	durationColumn, ok := statusDecimalColumn(lines[0], "1234.56")
	if !ok {
		t.Fatalf("long duration missing from status lines: %#v", lines)
	}
	for _, want := range []string{"9.50"} {
		line := statusLineEndingWith(t, lines, want)
		got, ok := statusDecimalColumn(line, want)
		if !ok {
			t.Fatalf("duration %q missing from line %q", want, line)
		}
		if got != durationColumn {
			t.Fatalf("duration %q decimal column = %d, want %d\nlines:\n%s", want, got, durationColumn, strings.Join(lines, "\n"))
		}
	}
	if !strings.HasSuffix(lines[2], "      -") {
		t.Fatalf("missing duration placeholder should stay right-aligned:\n%s", strings.Join(lines, "\n"))
	}
}

// TestCompactStatusLinesAlignsSingleWidthStatusSymbols verifies checkmarks do not shift ASCII markers.
func TestCompactStatusLinesAlignsSingleWidthStatusSymbols(t *testing.T) {
	shortMinutes := 0.25
	longMinutes := 20.85
	view := statusView{
		Rows: []statusViewRow{
			{Name: "执行阶段", Kind: "stage", SessionID: "019ed960-a5c1-7691-80b9-c816fd4cf13d", Marker: "✓", DurationMinutes: &longMinutes},
			{Name: "修正阶段", Kind: "stage", SessionID: "019ed97b-d383-7732-97c5-0402a47579d6", Marker: "/", DurationMinutes: &shortMinutes},
		},
	}

	lines := compactStatusLines(view)
	if !statusDurationDecimalColumnsAligned(lines) {
		t.Fatalf("duration decimals should align across status symbols:\n%s", strings.Join(lines, "\n"))
	}
	if got := statusDisplayWidth("✓"); got != 1 {
		t.Fatalf("checkmark display width = %d, want 1", got)
	}
	if got := statusDisplayWidth("执行"); got != 4 {
		t.Fatalf("CJK display width = %d, want 4", got)
	}
}

// TestStageDurationUsesUUIDv7SessionStart verifies resumed stages do not lose leading digits in elapsed time.
func TestStageDurationUsesUUIDv7SessionStart(t *testing.T) {
	finishedAt := time.Date(2026, 6, 18, 12, 30, 0, 0, time.UTC)
	sessionStartedAt := finishedAt.Add(-22*time.Minute - 600*time.Millisecond)
	timingStartedAt := finishedAt.Add(-2*time.Minute - 600*time.Millisecond)
	state := statusViewImplementationContextState()
	state.Status = statusRunning
	state.Stage = "review_1"
	state.Stages["execution"] = "completed"
	state.Sessions = map[string]string{
		sessionStateKey("codex", "executor"): statusTestUUIDv7(sessionStartedAt),
	}
	state.StageTimings = map[string]StageTiming{
		"execution": {
			StartedAt:  timingStartedAt.Format(time.RFC3339Nano),
			FinishedAt: finishedAt.Format(time.RFC3339Nano),
		},
	}

	row := statusStageRow(t.TempDir(), state, compactStageSpecs[1], finishedAt)
	if row.DurationMinutes == nil {
		t.Fatalf("execution duration missing")
	}
	if got, want := *row.DurationMinutes, 22.01; got < want-0.001 || got > want+0.001 {
		t.Fatalf("execution duration = %.2f, want %.2f", got, want)
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
		if line == "  提示: 当前 run 的 lock 已失效，可运行 oz flow restart 重试当前阶段" {
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
	if !strings.Contains(joined, "oz flow restart -b1 重试当前批量阶段") {
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
	if !strings.Contains(joined, "oz flow restart -b2 删除失败记录并继续该批量任务") {
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
	if payload.Observability.DisplayID != "w1" {
		t.Fatalf("unexpected observability identity: %#v", payload.Observability)
	}
	if bytes.Contains(data, []byte(`"engine"`)) {
		t.Fatalf("observability must not expose internal engine: %s", data)
	}
	for _, row := range payload.Observability.Rows {
		if row.Kind == "stage" && row.Name == "执行阶段" && row.Marker == "→" {
			return
		}
	}
	t.Fatalf("runner observability missing running execution row: %s", data)
}

// statusViewImplementationContextState returns a minimal execution state for compact status tests.
func statusViewImplementationContextState() State {
	workflow := DefaultWorkflowConfig()
	workflow.Engine = "go-dag"
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

// statusLineEndingWith returns the first compact line whose final visible column matches value.
func statusLineEndingWith(t *testing.T, lines []string, value string) string {
	t.Helper()
	for _, line := range lines {
		if strings.HasSuffix(strings.TrimRight(line, " "), value) {
			return line
		}
	}
	t.Fatalf("line ending with %q not found in %#v", value, lines)
	return ""
}

// statusValueColumn returns the terminal display column for the last occurrence of value.
func statusValueColumn(line, value string) (int, bool) {
	index := strings.LastIndex(line, value)
	if index < 0 {
		return 0, false
	}
	return statusDisplayWidth(line[:index]), true
}

// statusDecimalColumn returns the display column of the decimal point in a terminal duration value.
func statusDecimalColumn(line, value string) (int, bool) {
	valueColumn, ok := statusValueColumn(line, value)
	if !ok {
		return 0, false
	}
	decimalIndex := strings.Index(value, ".")
	if decimalIndex < 0 {
		return 0, false
	}
	return valueColumn + statusDisplayWidth(value[:decimalIndex]), true
}

// statusTestUUIDv7 returns a minimal UUIDv7-shaped session id with the given timestamp.
func statusTestUUIDv7(startedAt time.Time) string {
	timestamp := fmt.Sprintf("%012x", startedAt.UnixMilli())
	return timestamp[:8] + "-" + timestamp[8:12] + "-7abc-8000-000000000000"
}
