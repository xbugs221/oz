// Package app defines the status view model and maps workflow state into rows.
package app

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type statusView struct {
	DisplayID      string            `json:"display_id"`
	Indicator      string            `json:"-"`
	RunStatus      string            `json:"-"`
	Engine         string            `json:"engine,omitempty"`
	Rows           []statusViewRow   `json:"rows"`
	Artifacts      map[string]string `json:"artifacts"`
	RunArtifactDir string            `json:"-"`
	WallMinutes    *float64          `json:"-"`
}

type statusViewRow struct {
	Kind            string            `json:"kind"`
	Name            string            `json:"name"`
	FullName        string            `json:"full_name"`
	Stage           string            `json:"stage"`
	Group           string            `json:"group,omitempty"`
	SessionID       string            `json:"session_id"`
	Marker          string            `json:"marker"`
	DurationMinutes *float64          `json:"duration_minutes,omitempty"`
	Indent          int               `json:"indent"`
	Artifacts       map[string]string `json:"artifacts,omitempty"`
}

type compactStageSpec struct {
	role   string
	stage  string
	name   string
	prefix string
}

var compactStageSpecs = []compactStageSpec{
	{role: "planner", stage: "planning", name: "规划阶段", prefix: "planning"},
	{role: "executor", stage: "execution", name: "执行阶段", prefix: "execution"},
	{role: "reviewer", stage: "review_1", name: "审核阶段", prefix: "review_"},
	{role: "fixer", stage: "fix_1", name: "修正阶段", prefix: "fix_"},
	{role: "qa", stage: "qa_1", name: "测试阶段", prefix: "qa_"},
	{role: "archiver", stage: "archive", name: "归档阶段", prefix: "archive"},
}

// buildStatusView converts durable workflow state into one reusable compact view.
func buildStatusView(repo string, state State, displayID, runningMarker string) statusView {
	state = humanDisplayState(repo, state)
	ensureWorkflowConfig(&state)
	normalizeStateMaps(&state)
	now := time.Now().UTC()
	view := statusView{
		DisplayID:      nonEmpty(displayID, state.RunID),
		Indicator:      runningMarker,
		RunStatus:      state.Status,
		Engine:         statusViewEngine(state),
		RunArtifactDir: runDir(repo, state.RunID),
		Artifacts:      statusRootArtifacts(repo, state),
		WallMinutes:    statusWorkflowWallDuration(state, now),
	}
	for _, spec := range compactStageSpecs {
		row := statusStageRow(repo, state, spec, now)
		if runningMarker != "" && row.Marker == "→" {
			row.Marker = runningMarker
		}
		view.Rows = append(view.Rows, row)
	}
	applyStatusRunningMarker(&view, runningMarker)
	return view
}

// buildHumanStatusView builds the compact human view without internal parallel fan-in rows.
func buildHumanStatusView(repo string, state State, displayID, runningMarker string) statusView {
	state = humanDisplayState(repo, state)
	ensureWorkflowConfig(&state)
	normalizeStateMaps(&state)
	now := time.Now().UTC()
	view := statusView{
		DisplayID:      nonEmpty(displayID, state.RunID),
		Indicator:      runningMarker,
		RunStatus:      state.Status,
		Engine:         statusViewEngine(state),
		RunArtifactDir: runDir(repo, state.RunID),
		Artifacts:      statusRootArtifacts(repo, state),
		WallMinutes:    statusWorkflowWallDuration(state, now),
	}
	for _, spec := range compactStageSpecs {
		row := statusStageRow(repo, state, spec, now)
		if runningMarker != "" && row.Marker == "→" {
			row.Marker = runningMarker
		}
		view.Rows = append(view.Rows, row)
	}
	applyStatusRunningMarker(&view, runningMarker)
	return view
}

// statusViewEngine keeps the internal engine out of public JSON observability.
func statusViewEngine(state State) string {
	return ""
}

// statusStageRow builds one main-stage row, aggregating repeated review or QA rounds.
func statusStageRow(repo string, state State, spec compactStageSpec, now time.Time) statusViewRow {
	stages := matchingStatusStages(state, spec)
	row := statusViewRow{
		Kind:      "stage",
		Name:      spec.name,
		FullName:  spec.stage,
		Stage:     spec.stage,
		SessionID: statusRoleSessionID(state, spec.role),
		Marker:    statusStageMarker(state, stages),
		Artifacts: map[string]string{"stage_artifact": statusStageArtifact(repo, state, statusStageArtifactStage(state, spec, stages))},
	}
	if state.Status == statusBlocked && spec.role == "reviewer" {
		row.Marker = "x"
	}
	if state.Status == statusValidationBlocked && spec.role == "qa" {
		row.Marker = "x"
	}
	if state.Status == statusAcceptanceContractBlocked && spec.role == "executor" {
		row.Marker = "x"
	}
	if minutes, ok := statusStageDuration(state, stages, row.SessionID, now); ok {
		row.DurationMinutes = &minutes
	}
	return row
}

// statusStageArtifactStage chooses the concrete iteration represented by a compact row artifact.
func statusStageArtifactStage(state State, spec compactStageSpec, stages []string) string {
	if spec.prefix == spec.stage {
		return spec.stage
	}
	if strings.HasPrefix(state.Stage, spec.prefix) {
		return state.Stage
	}
	if len(stages) > 0 {
		return stages[len(stages)-1]
	}
	return spec.stage
}

// matchingStatusStages returns concrete durable stages represented by one compact row.
func matchingStatusStages(state State, spec compactStageSpec) []string {
	if spec.prefix == spec.stage {
		return []string{spec.stage}
	}
	stages := workflowStagesForState(state)
	var out []string
	for _, stage := range stages {
		if strings.HasPrefix(stage, spec.prefix) && statusStageReached(state, stage) {
			out = append(out, stage)
		}
	}
	if len(out) == 0 {
		return []string{spec.stage}
	}
	return out
}

// statusStageReached reports whether an iterated stage has durable progress in this run.
func statusStageReached(state State, stage string) bool {
	if state.Stage == stage {
		return true
	}
	if _, ok := state.Stages[stage]; ok {
		return true
	}
	if _, ok := state.StageTimings[stage]; ok {
		return true
	}
	if _, ok := state.DAGNodes[stage]; ok {
		return true
	}
	return false
}

// statusRoleSessionID returns the visible session id for a compact role row.
func statusRoleSessionID(state State, role string) string {
	if role == "planner" {
		return plannerSessionID(state)
	}
	stages := workflowStagesForState(state)
	if id := sessionRoleID(state, role, stages, nil); id != "" {
		return id
	}
	for _, key := range []string{
		sessionStateKey("codex", role),
		sessionStateKey("pi", role),
		sessionStateKey("agy", role),
		role,
	} {
		if id := state.Sessions[key]; id != "" {
			return id
		}
	}
	return ""
}

// statusStageMarker converts durable stage state into the compact progress marker.
func statusStageMarker(state State, stages []string) string {
	completed := 0
	running := false
	failed := false
	for _, stage := range stages {
		switch statusStageProgress(state, stage) {
		case "completed":
			completed++
		case statusRunning:
			running = true
		case statusFailed:
			failed = true
		}
	}
	if completed == 0 && !running && !failed {
		return "-"
	}
	return statusCountedMarker(completed, running, failed)
}

// statusStageProgress merges scheduler, DAG, and timing evidence for one compact stage marker.
func statusStageProgress(state State, stage string) string {
	if state.Stages[stage] == "completed" {
		return "completed"
	}
	if state.Stage == stage && state.Status == statusRunning {
		return statusRunning
	}
	if state.Stage == stage && state.Status != "" && state.Status != statusRunning && state.Status != statusDone {
		return statusFailed
	}
	if node, ok := state.DAGNodes[stage]; ok {
		if statusDAGNodeSucceeded(node.Status) {
			return "completed"
		}
		switch node.Status {
		case statusRunning:
			return statusRunning
		case statusFailed, "error":
			return statusFailed
		}
	}
	if timing, ok := state.StageTimings[stage]; ok && timing.StartedAt != "" && timing.FinishedAt != "" {
		return "completed"
	}
	return ""
}

// applyStatusRunningMarker replaces every running marker in status/watch rows.
func applyStatusRunningMarker(view *statusView, runningMarker string) {
	if runningMarker == "" || runningMarker == "→" {
		return
	}
	for i := range view.Rows {
		view.Rows[i].Marker = strings.ReplaceAll(view.Rows[i].Marker, "→", runningMarker)
	}
}

// statusDAGNodeSucceeded normalizes durable DAG node success values used across runners.
func statusDAGNodeSucceeded(status string) bool {
	return status == "success" || status == "completed" || status == statusDone
}

// statusStageDuration sums timing records for the concrete stages in one compact row.
func statusStageDuration(state State, stages []string, sessionID string, now time.Time) (float64, bool) {
	total := 0.0
	found := false
	sessionStartedAt, hasSessionStartedAt := statusUUIDv7StartedAt(sessionID)
	for _, stage := range stages {
		timing, ok := state.StageTimings[stage]
		if !ok || timing.StartedAt == "" {
			continue
		}
		startedAt, err := time.Parse(time.RFC3339Nano, timing.StartedAt)
		if err != nil {
			continue
		}
		if hasSessionStartedAt && sessionStartedAt.Before(startedAt) {
			startedAt = sessionStartedAt
		}
		finishedAt := now
		if timing.FinishedAt != "" {
			finishedAt, err = time.Parse(time.RFC3339Nano, timing.FinishedAt)
			if err != nil {
				continue
			}
		} else if state.Stage != stage || state.Status != statusRunning {
			continue
		}
		if finishedAt.Before(startedAt) {
			continue
		}
		total += finishedAt.Sub(startedAt).Minutes()
		found = true
	}
	return total, found
}

// statusUUIDv7StartedAt extracts the embedded millisecond timestamp from UUIDv7 session ids.
func statusUUIDv7StartedAt(sessionID string) (time.Time, bool) {
	compact := strings.ReplaceAll(sessionID, "-", "")
	if len(compact) < 13 || compact[12] != '7' {
		return time.Time{}, false
	}
	millis, err := strconv.ParseInt(compact[:12], 16, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.UnixMilli(millis).UTC(), true
}

// statusStageArtifact returns the fixed artifact path for one main workflow stage.
func statusStageArtifact(repo string, state State, stage string) string {
	base := runDir(repo, state.RunID)
	switch stage {
	case "planning":
		return filepath.Join(repo, "docs", "changes", state.ChangeName, "proposal.md")
	case "execution":
		return filepath.Join(repo, "docs", "changes", state.ChangeName, "task.md")
	case "archive":
		return filepath.Join(base, "delivery-summary.md")
	}
	if strings.HasPrefix(stage, "review_") {
		return filepath.Join(base, "review-"+strings.TrimPrefix(stage, "review_")+".json")
	}
	if strings.HasPrefix(stage, "fix_") {
		return filepath.Join(base, "fix-"+strings.TrimPrefix(stage, "fix_")+"-summary.md")
	}
	if strings.HasPrefix(stage, "qa_") {
		return filepath.Join(base, "qa-"+strings.TrimPrefix(stage, "qa_")+".json")
	}
	return base
}

// statusRootArtifacts returns fixed run and change artifact paths for JSON observability.
func statusRootArtifacts(repo string, state State) map[string]string {
	changeDir := filepath.Join(repo, "docs", "changes", state.ChangeName)
	return map[string]string{
		"run_state":         filepath.Join(runDir(repo, state.RunID), "state.json"),
		"change_proposal":   filepath.Join(changeDir, "proposal.md"),
		"change_design":     filepath.Join(changeDir, "design.md"),
		"change_spec":       filepath.Join(changeDir, "spec.md"),
		"change_task":       filepath.Join(changeDir, "task.md"),
		"change_acceptance": filepath.Join(changeDir, "acceptance.json"),
	}
}
