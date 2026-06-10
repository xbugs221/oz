// Package app builds the shared compact status view used by human status/watch output and JSON observability.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type statusView struct {
	DisplayID      string            `json:"display_id"`
	Indicator      string            `json:"-"`
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
		Engine:         statusViewEngine(state),
		RunArtifactDir: runDir(repo, state.RunID),
		Artifacts:      statusRootArtifacts(repo, state),
	}
	for _, spec := range compactStageSpecs {
		row := statusStageRow(repo, state, spec)
		if runningMarker != "" && row.Marker == "→" {
			row.Marker = runningMarker
		}
		view.Rows = append(view.Rows, row)
		view.Rows = append(view.Rows, statusSubagentRows(repo, state, spec.stage)...)
	}
	return view
}

// buildHumanStatusView adds human-only parallel fan-in summaries to the compact view.
func buildHumanStatusView(repo string, state State, displayID, runningMarker string) statusView {
	ensureWorkflowConfig(&state)
	normalizeStateMaps(&state)
	view := statusView{
		DisplayID:      nonEmpty(displayID, state.RunID),
		Indicator:      runningMarker,
		Engine:         statusViewEngine(state),
		RunArtifactDir: runDir(repo, state.RunID),
		Artifacts:      statusRootArtifacts(repo, state),
	}
	for _, spec := range compactStageSpecs {
		row := statusStageRow(repo, state, spec)
		if runningMarker != "" && row.Marker == "→" {
			row.Marker = runningMarker
		}
		view.Rows = append(view.Rows, row)
		view.Rows = append(view.Rows, statusParallelGroupRows(repo, state, spec.stage)...)
		view.Rows = append(view.Rows, statusSubagentRows(repo, state, spec.stage)...)
	}
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
	if state.Status == statusBlocked && spec.role == "reviewer" {
		row.Marker = "x"
	}
	if state.Status == statusValidationBlocked && spec.role == "qa" {
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
		sessionStateKey("opencode", role),
		sessionStateKey("pi", role),
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
	for _, stage := range stages {
		if state.Stage == stage && state.Status == statusRunning && state.Stages[stage] != "completed" {
			return "→"
		}
	}
	for _, stage := range stages {
		if state.Stage == stage && state.Status != "" && state.Status != statusRunning && state.Status != statusDone {
			return "x"
		}
	}
	completed := 0
	for _, stage := range stages {
		if state.Stages[stage] == "completed" {
			completed++
		}
	}
	if completed > 0 {
		return strings.Repeat("✓", completed)
	}
	return "-"
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

// statusParallelGroupRows summarizes configured fan-in artifacts for human status.
func statusParallelGroupRows(repo string, state State, parentStage string) []statusViewRow {
	if !state.Workflow.Parallel.Enabled || !statusParallelParentReached(state, parentStage) {
		return nil
	}
	var rows []statusViewRow
	for _, groupName := range statusGroupsForStage(parentStage) {
		group, ok := state.Workflow.Parallel.Groups[groupName]
		if !ok || len(group.Members) == 0 {
			continue
		}
		iteration, err := statusGroupIteration(parentStage, groupName)
		if err != nil {
			continue
		}
		groupArtifact := parallelArtifactPath(runDir(repo, state.RunID), groupName, iteration)
		total := len(group.Members)
		artifact, err := ReadParallelArtifact(groupArtifact)
		if err != nil {
			rows = append(rows, statusParallelGroupRow(parentStage, groupName, groupArtifact, fmt.Sprintf("0/%d", total), "missing "+filepath.Base(groupArtifact)))
			if !os.IsNotExist(err) {
				rows[len(rows)-1].Marker = "invalid " + filepath.Base(groupArtifact)
			}
			continue
		}
		if err := ValidateParallelArtifactForGroup(artifact, groupName, group); err != nil {
			rows = append(rows, statusParallelGroupRow(parentStage, groupName, groupArtifact, fmt.Sprintf("0/%d", total), "invalid "+filepath.Base(groupArtifact)))
			continue
		}
		successCount := 0
		for _, member := range artifact.Members {
			if memberStatusSucceeded(member.Status) {
				successCount++
			}
		}
		status := "failed"
		if successCount == total {
			status = "success"
		}
		rows = append(rows, statusParallelGroupRow(parentStage, groupName, groupArtifact, fmt.Sprintf("%d/%d", successCount, total), status))
		for _, member := range artifact.Members {
			rows = append(rows, statusViewRow{
				Kind:      "parallel_member",
				Name:      member.Name,
				FullName:  member.Name + ":parallel-summary",
				Stage:     parentStage,
				Group:     groupName,
				SessionID: member.Status,
				Marker:    "",
				Indent:    4,
				Artifacts: map[string]string{
					"group_artifact": groupArtifact,
				},
			})
		}
	}
	return rows
}

// statusParallelParentReached reports whether a parent stage should expose configured helper artifacts.
func statusParallelParentReached(state State, parentStage string) bool {
	if statusStageReached(state, parentStage) {
		return true
	}
	if parentStage == "execution" && state.Stages["execution"] == "completed" {
		return true
	}
	return false
}

// statusParallelGroupRow builds the single fan-in summary line for a helper group.
func statusParallelGroupRow(parentStage, groupName, groupArtifact, progress, marker string) statusViewRow {
	return statusViewRow{
		Kind:      "parallel_group",
		Name:      "并行 " + groupName,
		FullName:  groupName + ":parallel-summary",
		Stage:     parentStage,
		Group:     groupName,
		SessionID: progress,
		Marker:    marker,
		Indent:    2,
		Artifacts: map[string]string{
			"group_artifact": groupArtifact,
		},
	}
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
	candidates := []string{fmt.Sprintf("%s_%d", groupName, index+1)}
	if groupName == "implementation_context" {
		candidates = append(candidates, fmt.Sprintf("before_execution_%d", index+1))
	}
	if iteration > 0 {
		candidates = append(candidates, fmt.Sprintf("%s_%d_%d", groupName, iteration, index+1))
		if visualGroup := statusVisualGroupName(groupName); visualGroup != groupName {
			candidates = append(candidates, fmt.Sprintf("%s_%d_%d", visualGroup, iteration, index+1))
		}
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
		sessionStateKey("opencode", "subagent:"+groupName+":"+member.Name+":"+strconv.Itoa(iteration)),
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
	var lines []string
	for _, row := range view.Rows {
		prefix := strings.Repeat(" ", row.Indent)
		name := row.Name
		if row.Kind == "parallel_group" || row.Kind == "parallel_member" {
			name = "- " + name
		}
		lines = append(lines, fmt.Sprintf("%s%s %s %s %s", prefix, name, statusText(row.SessionID), statusText(row.Marker), statusDurationText(row.DurationMinutes)))
	}
	return lines
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
