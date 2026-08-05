// Package app defines the status view model and maps workflow state into rows.
package app

import (
	"path/filepath"
	"sort"
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
	{role: "executor", stage: "execution", name: humanWorkflowStageName("execution"), prefix: "execution"},
	{role: "repairer", stage: "repair_1", name: "优化", prefix: "repair_"},
	{role: "reviewer", stage: "review_1", name: "审核阶段", prefix: "review_"},
	{role: "fixer", stage: "fix_1", name: "修正阶段", prefix: "fix_"},
	{role: "qa", stage: "qa_1", name: humanWorkflowStageName("qa_1"), prefix: "qa_"},
	{role: "archiver", stage: "archive", name: humanWorkflowStageName("archive"), prefix: "archive"},
}

var qualityLoopCompactStageSpecs = []compactStageSpec{
	{role: "repairer", stage: "audit_1", name: humanWorkflowStageName("audit_1"), prefix: "audit_"},
	{role: "repairer", stage: "targeted_repair_1", name: humanWorkflowStageName("targeted_repair_1"), prefix: "targeted_repair_"},
	{role: "qa", stage: "qa_1", name: humanWorkflowStageName("qa_1"), prefix: "qa_"},
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
	for _, spec := range statusStageSpecs(state) {
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
	for _, spec := range statusStageSpecs(state) {
		row := statusStageRow(repo, state, spec, now)
		if runningMarker != "" && row.Marker == "→" {
			row.Marker = runningMarker
		}
		view.Rows = append(view.Rows, row)
	}
	applyStatusRunningMarker(&view, runningMarker)
	return view
}

// statusStageSpecs selects only the stage generation sealed into the run snapshot.
func statusStageSpecs(state State) []compactStageSpec {
	specs := append([]compactStageSpec(nil), compactStageSpecs[:2]...)
	if state.Workflow.Generation == qualityLoopWorkflowGeneration {
		specs = append(specs, qualityLoopCompactStageSpecs...)
		return append(specs, compactStageSpecs[6])
	}
	if usesRepairWorkflow(state.Workflow) {
		if state.Workflow.MaxRepairIterations > 0 {
			specs = append(specs, compactStageSpecs[2])
		}
		specs = append(specs, compactStageSpecs[5])
	} else if state.Workflow.MaxReviewIterations > 0 {
		specs = append(specs, compactStageSpecs[3], compactStageSpecs[4], compactStageSpecs[5])
	}
	return append(specs, compactStageSpecs[6])
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
	if state.Workflow.Generation == qualityLoopWorkflowGeneration && isQualityLoopBlockedState(state) {
		if qualityLoopBlockedSpec(state).prefix == spec.prefix {
			row.Marker = "x"
		}
	} else if state.Status == statusBlocked && spec.role == blockedWorkflowRole(state) {
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

// isQualityLoopBlockedState reports the two recoverable quality-loop pause states.
func isQualityLoopBlockedState(state State) bool {
	return state.Status == statusBlockedEnvironment || state.Stage == statusBlockedEnvironment ||
		state.Status == statusBlockedStalled || state.Stage == statusBlockedStalled
}

// qualityLoopBlockedSpec identifies the most recently active repair mode for a blocked loop.
func qualityLoopBlockedSpec(state State) compactStageSpec {
	targeted := qualityLoopCompactStageSpecs[1]
	audit := qualityLoopCompactStageSpecs[0]
	qa := qualityLoopCompactStageSpecs[2]
	archive := compactStageSpecs[6]
	switch {
	case strings.HasPrefix(state.QualityLoop.BlockedFromStage, targeted.prefix):
		return targeted
	case strings.HasPrefix(state.QualityLoop.BlockedFromStage, audit.prefix):
		return audit
	case strings.HasPrefix(state.QualityLoop.BlockedFromStage, qa.prefix):
		return qa
	case state.QualityLoop.BlockedFromStage == archive.stage:
		return archive
	}
	var latestTargeted, latestAudit, latestQA, latestArchive time.Time
	for stage, timing := range state.StageTimings {
		startedAt, err := time.Parse(time.RFC3339Nano, timing.StartedAt)
		if err != nil {
			continue
		}
		switch {
		case strings.HasPrefix(stage, targeted.prefix) && startedAt.After(latestTargeted):
			latestTargeted = startedAt
		case strings.HasPrefix(stage, audit.prefix) && startedAt.After(latestAudit):
			latestAudit = startedAt
		case strings.HasPrefix(stage, qa.prefix) && startedAt.After(latestQA):
			latestQA = startedAt
		case stage == archive.stage && startedAt.After(latestArchive):
			latestArchive = startedAt
		}
	}
	latest := latestAudit
	spec := audit
	for _, candidate := range []struct {
		started time.Time
		spec    compactStageSpec
	}{
		{started: latestTargeted, spec: targeted},
		{started: latestQA, spec: qa},
		{started: latestArchive, spec: archive},
	} {
		if candidate.started.After(latest) {
			latest = candidate.started
			spec = candidate.spec
		}
	}
	if !latest.IsZero() {
		return spec
	}
	for stage := range state.Stages {
		if strings.HasPrefix(stage, targeted.prefix) {
			return targeted
		}
	}
	return audit
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
	stages := observedStatusStages(state)
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
	stages := observedStatusStages(state)
	if id := sessionRoleID(state, role, stages, nil); id != "" {
		return id
	}
	for _, key := range []string{
		sessionStateKey("codex", role),
		sessionStateKey("pi", role),
		sessionStateKey("agy", role),
		sessionStateKey("claude", role),
		role,
	} {
		if id := state.Sessions[key]; id != "" {
			return id
		}
	}
	return ""
}

// observedStatusStages preserves finite snapshots and merges only dynamic quality-loop instances.
func observedStatusStages(state State) []string {
	stages := append([]string(nil), workflowStagesForState(state)...)
	if !usesQualityLoop(state.Workflow) {
		return stages
	}
	seen := make(map[string]bool, len(stages))
	for _, stage := range stages {
		seen[stage] = true
	}
	add := func(stage string) {
		if stage == "" || seen[stage] || !isDynamicQualityLoopStage(stage) {
			return
		}
		seen[stage] = true
		stages = append(stages, stage)
	}
	add(state.Stage)
	for stage := range state.Stages {
		add(stage)
	}
	for stage := range state.StageTimings {
		add(stage)
	}
	for stage := range state.DAGNodes {
		add(stage)
	}
	sort.SliceStable(stages, func(i, j int) bool {
		leftRank, leftIteration := qualityLoopStageOrder(stages[i])
		rightRank, rightIteration := qualityLoopStageOrder(stages[j])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if leftIteration != rightIteration {
			return leftIteration < rightIteration
		}
		return stages[i] < stages[j]
	})
	return stages
}

// isDynamicQualityLoopStage recognizes persisted audit, QA, and targeted-repair instances.
func isDynamicQualityLoopStage(stage string) bool {
	rank, iteration := qualityLoopStageOrder(stage)
	return rank >= 1 && rank <= 3 && iteration > 0
}

// qualityLoopStageOrder returns display phase and numeric iteration for a stage.
func qualityLoopStageOrder(stage string) (int, int) {
	prefixes := []string{"audit_", "qa_", "targeted_repair_"}
	for index, prefix := range prefixes {
		if !strings.HasPrefix(stage, prefix) {
			continue
		}
		iteration, err := strconv.Atoi(strings.TrimPrefix(stage, prefix))
		if err == nil {
			return index + 1, iteration
		}
	}
	switch stage {
	case "planning":
		return -2, 0
	case "execution":
		return -1, 0
	case "archive":
		return 4, 0
	default:
		return 0, 0
	}
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
	if stage == workflowStageArchive && state.Stages[stage] != "" && state.Stages[stage] != statusRunning {
		return statusFailed
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
	mayApplySessionStart := hasSessionStartedAt
	for _, stage := range stages {
		timing, ok := state.StageTimings[stage]
		if !ok || timing.StartedAt == "" {
			continue
		}
		startedAt, err := time.Parse(time.RFC3339Nano, timing.StartedAt)
		if err != nil {
			continue
		}
		if mayApplySessionStart && sessionStartedAt.Before(startedAt) {
			startedAt = sessionStartedAt
		}
		mayApplySessionStart = false
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
		return filepath.Join(base, "state.json")
	case "archive":
		return filepath.Join(base, "delivery-summary.md")
	}
	if strings.HasPrefix(stage, "review_") {
		return filepath.Join(base, "review-"+strings.TrimPrefix(stage, "review_")+".json")
	}
	if strings.HasPrefix(stage, "repair_") {
		return filepath.Join(base, "repair-"+strings.TrimPrefix(stage, "repair_")+".json")
	}
	if strings.HasPrefix(stage, "audit_") {
		return filepath.Join(base, "audit-"+strings.TrimPrefix(stage, "audit_")+".json")
	}
	if strings.HasPrefix(stage, "targeted_repair_") {
		return filepath.Join(base, "targeted-repair-"+strings.TrimPrefix(stage, "targeted_repair_")+".json")
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
		"change_acceptance": filepath.Join(changeDir, "acceptance.json"),
	}
}
