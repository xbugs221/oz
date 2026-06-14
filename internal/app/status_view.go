// Package app builds the shared compact status view used by human status/watch output and JSON observability.
package app

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type statusView struct {
	DisplayID      string            `json:"display_id"`
	Indicator      string            `json:"-"`
	RunStatus      string            `json:"-"`
	Engine         string            `json:"engine"`
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
		stages := matchingStatusStages(state, spec)
		row := statusStageRow(repo, state, spec, now)
		if runningMarker != "" && row.Marker == "→" {
			row.Marker = runningMarker
		}
		view.Rows = append(view.Rows, row)
		view.Rows = append(view.Rows, statusSubagentRows(repo, state, statusStageArtifactStage(state, spec, stages), now)...)
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
		stages := matchingStatusStages(state, spec)
		row := statusStageRow(repo, state, spec, now)
		if runningMarker != "" && row.Marker == "→" {
			row.Marker = runningMarker
		}
		view.Rows = append(view.Rows, row)
		if spec.stage != "planning" {
			view.Rows = append(view.Rows, statusSubagentRows(repo, state, statusStageArtifactStage(state, spec, stages), now)...)
		}
	}
	applyStatusRunningMarker(&view, runningMarker)
	return view
}

// stageChecklistLines formats workflow status with optional runtime metadata.
func stageChecklistLines(state State, runtime map[string]stageRuntime) []string {
	return stageChecklistLinesForRepo("", state, runtime)
}

// stageChecklistLinesWithParallel formats human status lines with run-local parallel summaries.
func stageChecklistLinesWithParallel(repo string, state State, runtime map[string]stageRuntime) []string {
	return stageChecklistLinesForRepo(repo, state, runtime)
}

// stageChecklistLinesForRepo formats workflow status and optionally attaches run-local artifacts.
func stageChecklistLinesForRepo(repo string, state State, runtime map[string]stageRuntime) []string {
	engine := state.Engine
	if engine == "" {
		engine = state.Workflow.Engine
	}
	if engine == "" {
		engine = "go-dag"
	}
	var lines []string
	lines = append(lines, "- 引擎 "+engine)
	for _, item := range visibleSessionItems(state, runtime) {
		parts := []string{"-", item.label}
		if item.sessionID != "" {
			parts = append(parts, item.sessionID)
		}
		markers := strings.Repeat("✓", item.completed) + item.running
		if markers != "" {
			parts = append(parts, markers)
		}
		line := strings.Join(parts, " ")
		lines = append(lines, line)
		if repo != "" {
			lines = append(lines, parallelStatusLinesForRole(repo, state, item.role, "  ")...)
		}
	}
	if state.Status == statusBlocked || state.Stage == statusBlocked {
		reason := state.Error
		if reason == "" {
			reason = "审核修正达到上限，工作流已中断"
		}
		lines = append(lines, fmt.Sprintf("- 状态 %s x %s", statusBlocked, reason))
	}
	if state.Status == statusValidationBlocked || state.Stage == statusValidationBlocked {
		reason := state.Error
		if reason == "" {
			reason = "阶段验证达到上限，工作流已中断"
		}
		lines = append(lines, fmt.Sprintf("- 状态 %s x %s", statusValidationBlocked, reason))
	}
	if state.Status == statusAcceptanceContractBlocked || state.Stage == statusAcceptanceContractBlocked {
		reason := state.Error
		if reason == "" {
			reason = "验收合同预检未通过，工作流已中断"
		}
		lines = append(lines, fmt.Sprintf("- 状态 %s x %s", statusAcceptanceContractBlocked, reason))
	}
	if len(lines) == 0 {
		return []string{"- 写 未知 →"}
	}
	lines = append(lines, stageDurationSummaryLines(state, time.Now().UTC())...)
	return lines
}

type stageDurationItem struct {
	label   string
	stage   string
	minutes float64
}

// stageDurationSummaryLines formats human duration totals for executed stages only.
func stageDurationSummaryLines(state State, now time.Time) []string {
	items := stageDurationItems(state, now)
	if len(items) == 0 {
		return nil
	}
	parts := make([]string, 0, len(items))
	total := 0.0
	for _, item := range items {
		parts = append(parts, formatMinutes(item.minutes))
		total += item.minutes
	}
	lines := []string{fmt.Sprintf("- 耗时 %s分钟=%s", formatMinutes(total), strings.Join(parts, "+"))}
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("  - %s %s %s分钟", item.label, item.stage, formatMinutes(item.minutes)))
	}
	return lines
}

type stageDurationBucket struct {
	label     string
	stage     string
	minutes   float64
	stageName string
	count     int
}

// stageDurationItems collects valid timing records in workflow display order.
func stageDurationItems(state State, now time.Time) []stageDurationItem {
	if len(state.StageTimings) == 0 {
		return nil
	}
	var order []string
	buckets := map[string]*stageDurationBucket{}
	for _, stage := range workflowStagesForState(state) {
		timing, ok := state.StageTimings[stage]
		if !ok || timing.StartedAt == "" {
			continue
		}
		startedAt, err := time.Parse(time.RFC3339Nano, timing.StartedAt)
		if err != nil {
			continue
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
		duration := finishedAt.Sub(startedAt)
		if duration < 0 {
			continue
		}
		role, err := roleForStage(stage)
		if err != nil {
			continue
		}
		key := role.Name
		bucket, ok := buckets[key]
		if !ok {
			bucket = &stageDurationBucket{label: role.Label, stage: role.Name, stageName: stage}
			buckets[key] = bucket
			order = append(order, key)
		}
		bucket.minutes += duration.Minutes()
		bucket.count++
	}
	items := make([]stageDurationItem, 0, len(order))
	for _, key := range order {
		bucket := buckets[key]
		stage := bucket.stage
		if bucket.count == 1 {
			stage = bucket.stageName
		}
		items = append(items, stageDurationItem{
			label:   bucket.label,
			stage:   stage,
			minutes: bucket.minutes,
		})
	}
	return items
}

// formatMinutes renders minutes with at most two decimal places for status text.
func formatMinutes(minutes float64) string {
	text := strconv.FormatFloat(minutes, 'f', 2, 64)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ".")
	if text == "" || text == "-0" {
		return "0"
	}
	return text
}

// stageTool returns the configured backend name for session lookup.
func stageTool(state State, stage string) string {
	ensureWorkflowConfig(&state)
	if options, ok := state.Workflow.Stages[stage]; ok && options.Tool != "" {
		return options.Tool
	}
	return "codex"
}

// stageChecklistSignature identifies the currently visible workflow state.
func stageChecklistSignature(state State) string {
	return stageChecklistSignatureWithRuntime(state, nil)
}

// stageChecklistSignatureWithRuntime identifies visible workflow state for duplicate suppression.
func stageChecklistSignatureWithRuntime(state State, runtime map[string]stageRuntime) string {
	parts := []string{state.RunID, state.Status, state.Stage}
	for _, item := range visibleSessionItems(state, runtime) {
		parts = append(parts, item.role+":"+item.sessionID+":"+strconv.Itoa(item.completed)+":"+item.running)
	}
	return strings.Join(parts, "|")
}

type visibleSessionItem struct {
	role      string
	label     string
	sessionID string
	completed int
	running   string
}

const preWorkflowCompletedLabel = "工作流开始之前就已完成"

// visibleSessionItems groups launched workflow stages by human session role.
func visibleSessionItems(state State, runtime map[string]stageRuntime) []visibleSessionItem {
	stages := workflowStagesForState(state)
	var items []visibleSessionItem
	for _, workflowRole := range statusRoles() {
		role := workflowRole.Session
		if role == "planner" {
			item := visibleSessionItem{role: role, label: sessionRoleLabel(role), sessionID: plannerSessionID(state)}
			if item.sessionID == "" {
				item.sessionID = preWorkflowCompletedLabel
				item.completed = 1
			}
			items = append(items, item)
			continue
		}
		item := visibleSessionItem{role: role, label: sessionRoleLabel(role), sessionID: sessionRoleID(state, role, stages, runtime)}
		for _, stage := range stages {
			if stageSessionRole(stage) != role {
				continue
			}
			if state.Stages[stage] == "completed" {
				item.completed++
			}
			if stage == state.Stage && state.Status == statusRunning && state.Stages[stage] != "completed" {
				item.running = "→"
			}
		}
		occurred := roleOccurred(state, role, stages, runtime)
		if item.completed == 0 && item.running == "" && item.sessionID == "" && !occurred {
			continue
		}
		if role == "executor" && item.sessionID == "" && item.completed > 0 && item.running == "" {
			item.sessionID = preWorkflowCompletedLabel
		}
		if item.sessionID == "" && (item.completed > 0 || item.running != "" || occurred) {
			item.sessionID = "未知"
		}
		items = append(items, item)
	}
	return items
}

// plannerSessionID returns the planning session id from durable state.
func plannerSessionID(state State) string {
	ensureWorkflowConfig(&state)
	tool := "codex"
	if options, ok := state.Workflow.Stages["planning"]; ok && options.Tool != "" {
		tool = options.Tool
	}
	for _, key := range []string{sessionStateKey(tool, "planner"), sessionStateKey(tool, "planning"), "planner"} {
		if id := state.Sessions[key]; id != "" {
			return id
		}
	}
	return ""
}

// sessionRoleID returns the public session id for a grouped role line.
func sessionRoleID(state State, role string, stages []string, runtime map[string]stageRuntime) string {
	for _, stage := range stages {
		if stageSessionRole(stage) != role {
			continue
		}
		if meta, ok := runtime[stage]; ok {
			if meta.Thread != "" {
				return meta.Thread
			}
			if meta.Failed {
				return "失败"
			}
		}
	}
	for _, stage := range stages {
		if stageSessionRole(stage) != role {
			continue
		}
		if id := state.Sessions[sessionStateKey(stageTool(state, stage), role)]; id != "" {
			return id
		}
	}
	return ""
}

// roleOccurred reports whether any stage for a role has entered durable or live state.
func roleOccurred(state State, role string, stages []string, runtime map[string]stageRuntime) bool {
	for _, stage := range stages {
		if stageSessionRole(stage) != role {
			continue
		}
		if state.Stages[stage] != "" {
			return true
		}
		if state.Stage == stage && state.Status != "" && state.Status != statusDone {
			return true
		}
		if _, ok := runtime[stage]; ok {
			return true
		}
	}
	return false
}

// statusViewEngine reports the effective workflow engine for JSON observability.
func statusViewEngine(state State) string {
	if state.Engine != "" {
		return state.Engine
	}
	if state.Workflow.Engine != "" {
		return state.Workflow.Engine
	}
	return "go-dag"
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
	if spec.role == "planner" && row.Marker == "-" && statusPlanningContextCompleted(repo, state) {
		row.Marker = "✓"
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
	if minutes, ok := statusStageDuration(state, stages, now); ok {
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

// statusPlanningContextCompleted treats execution preflight context fan-in as the completed planning marker.
func statusPlanningContextCompleted(repo string, state State) bool {
	for _, id := range []string{"planning_context_fanin"} {
		if node, ok := state.DAGNodes[id]; ok && statusDAGNodeSucceeded(node.Status) {
			return true
		}
	}
	return fileExists(parallelArtifactPath(runDir(repo, state.RunID), "planning_context", 0))
}

// statusDAGNodeSucceeded normalizes durable DAG node success values used across runners.
func statusDAGNodeSucceeded(status string) bool {
	return status == "success" || status == "completed" || status == statusDone
}

// statusStageDuration sums timing records for the concrete stages in one compact row.
func statusStageDuration(state State, stages []string, now time.Time) (float64, bool) {
	total := 0.0
	found := false
	for _, stage := range stages {
		timing, ok := state.StageTimings[stage]
		if !ok || timing.StartedAt == "" {
			continue
		}
		startedAt, err := time.Parse(time.RFC3339Nano, timing.StartedAt)
		if err != nil {
			continue
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

// statusSubagentRows returns reached helper member rows owned by a compact parent stage.
func statusSubagentRows(repo string, state State, parentStage string, now time.Time) []statusViewRow {
	if !state.Workflow.Parallel.Enabled {
		return nil
	}
	var rows []statusViewRow
	for _, groupName := range statusGroupsForStage(parentStage) {
		group, ok := state.Workflow.Parallel.Groups[groupName]
		if !ok {
			continue
		}
		iteration, err := statusGroupIteration(parentStage, groupName)
		if err != nil {
			continue
		}
		groupArtifact := parallelArtifactPath(runDir(repo, state.RunID), groupName, iteration)
		for index, member := range group.Members {
			_, node, hasNode := statusSubagentNode(state, groupName, parentStage, iteration, index)
			sessionID := statusSubagentSessionID(state, groupName, member, iteration)
			memberArtifact := memberArtifactPath(repo, state.RunID, groupName, iteration, member.Name)
			if hasNode && node.Artifact != "" {
				memberArtifact = node.Artifact
			}
			if sessionID == "" && !hasNode && !fileExists(memberArtifact) {
				continue
			}
			row := statusViewRow{
				Kind:      "subagent",
				Name:      compactSubagentName(member.Name),
				FullName:  member.Name,
				Stage:     parentStage,
				Group:     groupName,
				SessionID: sessionID,
				Marker:    statusSubagentMarker(node, hasNode, memberArtifact),
				Indent:    2,
				Artifacts: map[string]string{
					"member_artifact": memberArtifact,
					"group_artifact":  groupArtifact,
				},
			}
			if minutes, ok := statusNodeDuration(node, now); ok {
				row.DurationMinutes = &minutes
			}
			rows = append(rows, row)
		}
	}
	return rows
}

// statusGroupsForStage maps compact parent stages to configured parallel helper groups.
func statusGroupsForStage(stage string) []string {
	if stage == "planning" {
		return []string{"planning_context"}
	}
	if stage == "execution" {
		return []string{"implementation_context"}
	}
	if strings.HasPrefix(stage, "review_") {
		return []string{"review"}
	}
	if strings.HasPrefix(stage, "qa_") {
		return []string{"qa"}
	}
	return nil
}

// statusGroupIteration returns the artifact iteration for one helper group.
func statusGroupIteration(stage, group string) (int, error) {
	if group == "review" {
		return stageIteration(stage)
	}
	if group == "qa" {
		return stageIteration(stage)
	}
	return 0, nil
}

// statusSubagentNode finds the DAG node for a configured member index.
func statusSubagentNode(state State, groupName, parentStage string, iteration, index int) (string, DAGNodeState, bool) {
	var candidates []string
	switch groupName {
	case "planning_context":
		candidates = append(candidates, fmt.Sprintf("%s_%d", groupName, index+1))
	case "implementation_context":
		candidates = append(candidates,
			fmt.Sprintf("implementation_context_%d", index+1),
			fmt.Sprintf("before_execution_%d", index+1),
		)
	case "review", "qa":
		if iteration > 0 {
			candidates = append(candidates, fmt.Sprintf("%s_%d_%d", statusVisualGroupName(groupName), iteration, index+1))
		}
	default:
		candidates = append(candidates, fmt.Sprintf("%s_%d", groupName, index+1))
	}
	for _, id := range candidates {
		if node, ok := state.DAGNodes[id]; ok {
			return id, node, true
		}
	}
	return "", DAGNodeState{}, false
}

// statusVisualGroupName maps configured helper groups to the DAG node prefix used by graph.go.
func statusVisualGroupName(groupName string) string {
	switch groupName {
	case "review":
		return "before_review"
	case "qa":
		return "before_qa"
	default:
		return groupName
	}
}

// statusSubagentSessionID returns the helper session id recorded by the subagent runner.
func statusSubagentSessionID(state State, groupName string, member ParallelMemberConfig, iteration int) string {
	tool := nonEmpty(member.Tool, "pi")
	keys := []string{
		sessionStateKey(tool, "subagent:"+groupName+":"+member.Name+":"+strconv.Itoa(iteration)),
		sessionStateKey("pi", "subagent:"+groupName+":"+member.Name+":"+strconv.Itoa(iteration)),
		sessionStateKey("codex", "subagent:"+groupName+":"+member.Name+":"+strconv.Itoa(iteration)),
	}
	for _, key := range keys {
		if id := state.Sessions[key]; id != "" {
			return id
		}
	}
	return ""
}

// statusSubagentMarker reports helper progress with strict artifact visibility for reached nodes.
func statusSubagentMarker(node DAGNodeState, hasNode bool, memberArtifact string) string {
	if !hasNode {
		if fileExists(memberArtifact) {
			return "✓"
		}
		return "-"
	}
	switch node.Status {
	case "success", "completed", statusDone:
		if node.Artifact != "" && !fileExists(node.Artifact) {
			return "x"
		}
		return "✓"
	case statusRunning:
		return "→"
	case statusFailed, "error":
		return "x"
	default:
		if fileExists(memberArtifact) {
			return "✓"
		}
		return "-"
	}
}

// statusNodeDuration returns a helper node wall-time duration once the node has started.
func statusNodeDuration(node DAGNodeState, now time.Time) (float64, bool) {
	if node.StartedAt == "" {
		return 0, false
	}
	startedAt, err := time.Parse(time.RFC3339Nano, node.StartedAt)
	if err != nil {
		return 0, false
	}
	finishedAt := now
	if node.FinishedAt != "" {
		finishedAt, err = time.Parse(time.RFC3339Nano, node.FinishedAt)
		if err != nil {
			return 0, false
		}
	} else if node.Status != statusRunning {
		return 0, false
	}
	if finishedAt.Before(startedAt) {
		return 0, false
	}
	return finishedAt.Sub(startedAt).Minutes(), true
}

// compactSubagentName shortens configured helper names for dense human output.
func compactSubagentName(name string) string {
	short := strings.TrimSpace(name)
	short = strings.ReplaceAll(short, "代码库", "代码")
	short = strings.ReplaceAll(short, "一致性", "")
	for _, suffix := range []string{"研究员", "审核员", "测试员", "采集员", "员"} {
		short = strings.TrimSuffix(short, suffix)
	}
	if strings.Contains(short, "风险") {
		short = "风险检查"
	}
	if utf8.RuneCountInString(short) <= 4 || isASCII(short) {
		return short
	}
	runes := []rune(short)
	return string(runes[:4])
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

// compactStatusLines renders the fixed-column human workflow rows for status and watch.
func compactStatusLines(view statusView) []string {
	rows := compactVisibleRows(view.Rows)
	widths := compactColumnWidths(rows)
	var lines []string
	for _, row := range rows {
		prefix := strings.Repeat(" ", row.Indent)
		name := compactHumanRowName(row)
		lines = append(lines, fmt.Sprintf("%s%s %s %s %s",
			prefix,
			padStatusColumn(name, widths.name-row.Indent),
			padStatusColumn(statusText(row.SessionID), widths.session),
			padStatusColumn(statusText(row.Marker), widths.marker),
			padStatusColumn(statusDurationText(row.DurationMinutes), widths.duration),
		))
	}
	return lines
}

type compactColumnWidth struct {
	name     int
	session  int
	marker   int
	duration int
}

// compactVisibleRows removes rows that add no signal to the human compact view.
func compactVisibleRows(rows []statusViewRow) []statusViewRow {
	out := make([]statusViewRow, 0, len(rows))
	for _, row := range rows {
		if row.Kind == "stage" && row.Stage == "planning" && row.SessionID == "" && (row.Marker == "-" || row.Marker == "✓") && row.DurationMinutes == nil {
			continue
		}
		out = append(out, row)
	}
	return out
}

// compactColumnWidths calculates terminal display widths so CJK names align in monospace output.
func compactColumnWidths(rows []statusViewRow) compactColumnWidth {
	widths := compactColumnWidth{}
	for _, row := range rows {
		name := compactHumanRowName(row)
		widths.name = max(widths.name, row.Indent+statusDisplayWidth(name))
		widths.session = max(widths.session, statusDisplayWidth(statusText(row.SessionID)))
		widths.marker = max(widths.marker, statusDisplayWidth(statusText(row.Marker)))
		widths.duration = max(widths.duration, statusDisplayWidth(statusDurationText(row.DurationMinutes)))
	}
	return widths
}

// compactHumanRowName shortens only terminal labels while preserving row names in JSON observability.
func compactHumanRowName(row statusViewRow) string {
	name := row.Name
	if row.Kind == "stage" {
		switch name {
		case "规划阶段":
			return "规划"
		case "执行阶段":
			return "执行"
		case "审核阶段":
			return "审核"
		case "修正阶段":
			return "修正"
		case "测试阶段":
			return "测试"
		case "归档阶段":
			return "归档"
		}
	}
	if row.Kind == "subagent" {
		return compactHumanSubagentName(row.FullName, name)
	}
	if row.Kind == "parallel_group" || row.Kind == "parallel_member" {
		return "- " + name
	}
	return name
}

// compactHumanSubagentName maps common helper roles to two-cell status labels.
func compactHumanSubagentName(fullName, fallback string) string {
	switch {
	case strings.Contains(fullName, "CLI/API"):
		return "CA"
	case strings.Contains(fullName, "浏览器"):
		return "浏览"
	case strings.Contains(fullName, "回归"):
		return "回归"
	case strings.Contains(fullName, "证据"):
		return "证据"
	case strings.Contains(fullName, "目标"):
		return "目标"
	case strings.Contains(fullName, "代码质量"):
		return "代码"
	case strings.Contains(fullName, "代码"):
		return "代码"
	case strings.Contains(fullName, "外部"):
		return "外部"
	case strings.Contains(fullName, "测试有效"):
		return "测试"
	case strings.Contains(fullName, "风险"):
		return "风险"
	case strings.Contains(fullName, "上下文"):
		return "上下"
	}
	if statusDisplayWidth(fallback) <= 4 {
		return fallback
	}
	runes := []rune(fallback)
	if len(runes) > 2 {
		return string(runes[:2])
	}
	return fallback
}

// statusHeaderText renders the proposal line with overall marker and workflow wall time.
func statusHeaderText(changeName string, view statusView) string {
	if view.WallMinutes == nil {
		return fmt.Sprintf("- %s %s -", changeName, compactOverallMarker(view))
	}
	return fmt.Sprintf("- %s %s %.2f 分钟", changeName, compactOverallMarker(view), *view.WallMinutes)
}

// compactOverallMarker reports a one-cell status indicator for the proposal header.
func compactOverallMarker(view statusView) string {
	switch view.RunStatus {
	case statusDone:
		return "✓"
	case statusFailed, statusBlocked, statusValidationBlocked, statusAcceptanceContractBlocked, statusInterrupted, statusStale:
		return "x"
	case statusRunning:
		if view.Indicator != "" {
			return view.Indicator
		}
		return "→"
	}
	if view.Indicator != "" {
		for _, row := range compactVisibleRows(view.Rows) {
			if strings.Contains(row.Marker, view.Indicator) {
				return view.Indicator
			}
		}
	}
	hasRunning := false
	hasFailed := false
	hasIncomplete := false
	for _, row := range compactVisibleRows(view.Rows) {
		marker := row.Marker
		if strings.Contains(marker, "x") {
			hasFailed = true
		}
		if strings.Contains(marker, "→") {
			hasRunning = true
		}
		if marker == "-" {
			hasIncomplete = true
		}
	}
	switch {
	case hasFailed:
		return "x"
	case hasRunning:
		return "→"
	case !hasIncomplete:
		return "✓"
	default:
		return "-"
	}
}

// statusWorkflowWallDuration measures elapsed workflow wall time from run creation to latest known activity.
func statusWorkflowWallDuration(state State, now time.Time) *float64 {
	startedAt, ok := statusWorkflowStartedAt(state)
	if !ok {
		return nil
	}
	finishedAt := now
	if state.Status != statusRunning {
		latest, found := statusLatestWorkflowActivity(state)
		if !found {
			return nil
		}
		finishedAt = latest
	}
	if finishedAt.Before(startedAt) {
		return nil
	}
	minutes := finishedAt.Sub(startedAt).Minutes()
	return &minutes
}

// statusWorkflowStartedAt finds the durable run start, preferring the sortable run id timestamp.
func statusWorkflowStartedAt(state State) (time.Time, bool) {
	if startedAt, err := time.Parse("20060102T150405.000000000Z", state.RunID); err == nil {
		return startedAt, true
	}
	return statusEarliestWorkflowActivity(state)
}

// statusEarliestWorkflowActivity returns the first persisted stage or DAG node timestamp.
func statusEarliestWorkflowActivity(state State) (time.Time, bool) {
	var earliest time.Time
	found := false
	visit := func(value string) {
		if value == "" {
			return
		}
		timestamp, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return
		}
		if !found || timestamp.Before(earliest) {
			earliest = timestamp
			found = true
		}
	}
	for _, timing := range state.StageTimings {
		visit(timing.StartedAt)
	}
	for _, node := range state.DAGNodes {
		visit(node.StartedAt)
	}
	return earliest, found
}

// statusLatestWorkflowActivity returns the last persisted stage or DAG node timestamp.
func statusLatestWorkflowActivity(state State) (time.Time, bool) {
	var latest time.Time
	found := false
	visit := func(value string) {
		if value == "" {
			return
		}
		timestamp, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return
		}
		if !found || timestamp.After(latest) {
			latest = timestamp
			found = true
		}
	}
	for _, timing := range state.StageTimings {
		visit(timing.FinishedAt)
		visit(timing.StartedAt)
	}
	for _, node := range state.DAGNodes {
		visit(node.FinishedAt)
		visit(node.StartedAt)
	}
	return latest, found
}

// humanDisplayState marks unowned running runs as stale without mutating durable state.
func humanDisplayState(repo string, state State) State {
	if isStaleRunningRun(repo, state) {
		state.Status = statusStale
	}
	return state
}

// isStaleRunningRun reports running state whose owner lock is no longer live.
func isStaleRunningRun(repo string, state State) bool {
	if state.Status != statusRunning || state.RunID == "" {
		return false
	}
	status, err := lockFileStatus(repo, state.RunID, runtime.GOOS)
	if err != nil {
		return false
	}
	return status == lockStatusStale
}

// statusCountedMarker compacts repeated completed rounds as ✓N before active or failed suffixes.
func statusCountedMarker(completed int, running, failed bool) string {
	var marker strings.Builder
	if completed == 1 {
		marker.WriteString("✓")
	} else if completed > 1 {
		marker.WriteString("✓")
		marker.WriteString(strconv.Itoa(completed))
	}
	if running {
		marker.WriteString("→")
	}
	if failed {
		marker.WriteString("x")
	}
	if marker.Len() == 0 {
		return "-"
	}
	return marker.String()
}

// padStatusColumn adds spaces based on display width instead of byte length.
func padStatusColumn(value string, width int) string {
	padding := width - statusDisplayWidth(value)
	if padding <= 0 {
		return value
	}
	return value + strings.Repeat(" ", padding)
}

// statusDisplayWidth approximates terminal cell width for ASCII and Chinese status text.
func statusDisplayWidth(value string) int {
	width := 0
	for _, r := range value {
		if r <= 127 {
			width++
			continue
		}
		width += 2
	}
	return width
}

// statusText renders an empty field as the required dash column.
func statusText(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

// statusDurationText renders minutes as two decimal places, or a dash when absent.
func statusDurationText(minutes *float64) string {
	if minutes == nil {
		return "-"
	}
	return fmt.Sprintf("%.2f", *minutes)
}

// isASCII reports whether a custom helper name is already a compact ASCII token.
func isASCII(value string) bool {
	for _, r := range value {
		if r > 127 {
			return false
		}
	}
	return true
}
