// Package app formats legacy status checklist lines and visible workflow sessions.
package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

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
	var lines []string
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
