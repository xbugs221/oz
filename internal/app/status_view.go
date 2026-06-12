// Package app builds the shared compact status view used by human status/watch output and JSON observability.
package app

import (
	"fmt"
	"path/filepath"
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
	ensureWorkflowConfig(&state)
	normalizeStateMaps(&state)
	view := statusView{
		DisplayID:      nonEmpty(displayID, state.RunID),
		Indicator:      runningMarker,
		RunStatus:      state.Status,
		Engine:         statusViewEngine(state),
		RunArtifactDir: runDir(repo, state.RunID),
		Artifacts:      statusRootArtifacts(repo, state),
	}
	for _, spec := range compactStageSpecs {
		stages := matchingStatusStages(state, spec)
		row := statusStageRow(repo, state, spec)
		if runningMarker != "" && row.Marker == "→" {
			row.Marker = runningMarker
		}
		view.Rows = append(view.Rows, row)
		view.Rows = append(view.Rows, statusSubagentRows(repo, state, statusStageArtifactStage(state, spec, stages))...)
	}
	applyStatusRunningMarker(&view, runningMarker)
	return view
}

// buildHumanStatusView builds the compact human view without internal parallel fan-in rows.
func buildHumanStatusView(repo string, state State, displayID, runningMarker string) statusView {
	ensureWorkflowConfig(&state)
	normalizeStateMaps(&state)
	view := statusView{
		DisplayID:      nonEmpty(displayID, state.RunID),
		Indicator:      runningMarker,
		RunStatus:      state.Status,
		Engine:         statusViewEngine(state),
		RunArtifactDir: runDir(repo, state.RunID),
		Artifacts:      statusRootArtifacts(repo, state),
	}
	for _, spec := range compactStageSpecs {
		stages := matchingStatusStages(state, spec)
		row := statusStageRow(repo, state, spec)
		if runningMarker != "" && row.Marker == "→" {
			row.Marker = runningMarker
		}
		view.Rows = append(view.Rows, row)
		if spec.stage != "planning" {
			view.Rows = append(view.Rows, statusSubagentRows(repo, state, statusStageArtifactStage(state, spec, stages))...)
		}
	}
	applyStatusRunningMarker(&view, runningMarker)
	return view
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
func statusStageRow(repo string, state State, spec compactStageSpec) statusViewRow {
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
	if minutes, ok := statusStageDuration(state, stages, time.Now().UTC()); ok {
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
func statusSubagentRows(repo string, state State, parentStage string) []statusViewRow {
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
			if minutes, ok := statusNodeDuration(node); ok {
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

// statusNodeDuration returns a helper node duration in minutes when both timestamps are valid.
func statusNodeDuration(node DAGNodeState) (float64, bool) {
	if node.StartedAt == "" || node.FinishedAt == "" {
		return 0, false
	}
	startedAt, err := time.Parse(time.RFC3339Nano, node.StartedAt)
	if err != nil {
		return 0, false
	}
	finishedAt, err := time.Parse(time.RFC3339Nano, node.FinishedAt)
	if err != nil || finishedAt.Before(startedAt) {
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
			statusDurationText(row.DurationMinutes),
		))
	}
	return lines
}

type compactColumnWidth struct {
	name    int
	session int
	marker  int
}

// compactVisibleRows removes rows that add no signal to the human compact view.
func compactVisibleRows(rows []statusViewRow) []statusViewRow {
	out := make([]statusViewRow, 0, len(rows))
	for _, row := range rows {
		if row.Kind == "stage" && row.Stage == "planning" && row.SessionID == "" && row.Marker == "✓" && row.DurationMinutes == nil {
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

// statusHeaderText renders the proposal line with overall marker and summed visible duration.
func statusHeaderText(changeName string, view statusView) string {
	total, ok := compactTotalDuration(view.Rows)
	if !ok {
		return fmt.Sprintf("- %s %s -", changeName, compactOverallMarker(view))
	}
	return fmt.Sprintf("- %s %s %.2f 分钟", changeName, compactOverallMarker(view), total)
}

// compactTotalDuration sums durations from visible rows only.
func compactTotalDuration(rows []statusViewRow) (float64, bool) {
	total := 0.0
	found := false
	for _, row := range compactVisibleRows(rows) {
		if row.DurationMinutes == nil {
			continue
		}
		total += *row.DurationMinutes
		found = true
	}
	return total, found
}

// compactOverallMarker reports a one-cell status indicator for the proposal header.
func compactOverallMarker(view statusView) string {
	switch view.RunStatus {
	case statusDone:
		return "✓"
	case statusFailed, statusBlocked, statusValidationBlocked, statusAcceptanceContractBlocked, statusInterrupted:
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
