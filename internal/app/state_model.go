// Package app contains workflow engine state and execution boundaries.
package app

import (
	"os"
	"strconv"
	"time"
)

const (
	internalGoDAGEngine             = "go-dag"
	publicWorkflowEngineLabel       = "内嵌工作流"
	statusRunning                   = "running"
	statusFailed                    = "failed"
	statusStale                     = "stale"
	statusDone                      = "done"
	statusAborted                   = "aborted_manual_intervention"
	statusArchived                  = "archived_superseded"
	statusInterrupted               = "interrupted"
	statusBlocked                   = "blocked_review_limit"
	statusValidationBlocked         = "blocked_validation_limit"
	statusAcceptanceContractBlocked = "blocked_acceptance_contract"
	errNoInitialCommit              = "首次 git commit 前不能启动 oz flow run，请创建初始提交后重试"
)

// publicEngineLabel returns the durable user-facing engine label for run state.
func publicEngineLabel(internalEngine string) string {
	if internalEngine == internalGoDAGEngine || internalEngine == "" {
		return publicWorkflowEngineLabel
	}
	return internalEngine
}

// stateUsesGoDAG reports whether a run should use the built-in DAG scheduler.
func stateUsesGoDAG(state State) bool {
	return state.Workflow.Engine == internalGoDAGEngine || state.Workflow.Engine == ""
}

// State is the durable source of truth for one sealed run.
type State struct {
	RunID               string                          `json:"run_id"`
	ChangeName          string                          `json:"change_name"`
	Sealed              bool                            `json:"sealed"`
	Status              string                          `json:"status"`
	Stage               string                          `json:"stage"`
	Engine              string                          `json:"engine,omitempty"`
	Error               string                          `json:"error"`
	BatchID             string                          `json:"batch_id,omitempty"`
	BatchIndex          int                             `json:"batch_index,omitempty"`
	BatchTotal          int                             `json:"batch_total,omitempty"`
	BaselineHead        string                          `json:"baseline_head"`
	BaselineDiff        string                          `json:"baseline_diff"`
	Sessions            map[string]string               `json:"sessions"`
	Stages              map[string]string               `json:"stages"`
	StageTimings        map[string]StageTiming          `json:"stage_timings,omitempty"`
	DAGNodes            map[string]DAGNodeState         `json:"dag_nodes,omitempty"`
	Processes           []ProcessState                  `json:"processes,omitempty"`
	Paths               map[string]string               `json:"paths"`
	Validation          map[string]StageValidationState `json:"validation,omitempty"`
	ArtifactGates       map[string]StageValidationState `json:"artifact_gates,omitempty"`
	AcceptanceRun       map[string]StageValidationState `json:"acceptance_run,omitempty"`
	AcceptancePreflight AcceptancePreflightState        `json:"acceptance_preflight,omitempty"`
	Workflow            WorkflowConfig                  `json:"workflow_config"`
}

// DAGNodeState records observable Go DAG node progress for human status and debugging.
type DAGNodeState struct {
	Status     string `json:"status"`
	Artifact   string `json:"artifact,omitempty"`
	StartedAt  string `json:"started_at,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
	Error      string `json:"error,omitempty"`
}

// ProcessState is the flattened process view consumed by external status renderers.
type ProcessState struct {
	Stage     string `json:"stage"`
	Role      string `json:"role"`
	Provider  string `json:"provider"`
	Status    string `json:"status"`
	SessionID string `json:"session_id,omitempty"`
}

// StageTiming records the wall-clock interval for one actually executed stage.
type StageTiming struct {
	StartedAt  string `json:"started_at,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
}

// LockInfo records conservative process ownership for a run lock file.
type LockInfo struct {
	PID       int    `json:"pid"`
	Hostname  string `json:"hostname"`
	RunID     string `json:"run_id"`
	StartedAt string `json:"started_at"`
}

func parallelGroupConfigured(workflow WorkflowConfig, name string) bool {
	group, ok := workflow.Parallel.Groups[name]
	return ok && len(group.Members) > 0
}

// refreshStateProcesses derives a stage-aware process list from sessions and DAG nodes.
func refreshStateProcesses(state *State) {
	if state == nil || state.RunID == "" {
		return
	}
	ensureWorkflowConfig(state)
	if !state.Workflow.Parallel.Enabled || len(state.Workflow.Parallel.Groups) == 0 {
		state.Processes = nil
		return
	}
	spec := BuildWorkflowSpec(state.ChangeName, state.Workflow)
	var processes []ProcessState
	for _, node := range spec.Nodes {
		if node.Type != "subagent" {
			continue
		}
		groupName := configGroupName(node.Group)
		group, ok := state.Workflow.Parallel.Groups[groupName]
		if !ok {
			continue
		}
		member, ok := parallelMemberByName(group, node.Member)
		if !ok {
			continue
		}
		provider := nonEmpty(member.Tool, "pi")
		role := "subagent:" + groupName + ":" + member.Name + ":" + strconv.Itoa(node.Iteration)
		sessionID := state.Sessions[sessionStateKey(provider, role)]
		if sessionID == "" && provider != "pi" {
			sessionID = state.Sessions[sessionStateKey("pi", role)]
		}
		nodeState, hasNode := state.DAGNodes[node.ID]
		status := processStatusFromNode(nodeState, hasNode, sessionID)
		if status == "" {
			continue
		}
		processes = append(processes, ProcessState{
			Stage:     nonEmpty(node.Stage, workflowNodeRunStage(node)),
			Role:      role,
			Provider:  provider,
			Status:    status,
			SessionID: sessionID,
		})
	}
	state.Processes = processes
}

// parallelMemberByName returns the configured helper member for process metadata.
func parallelMemberByName(group ParallelGroupConfig, name string) (ParallelMemberConfig, bool) {
	for _, member := range group.Members {
		if member.Name == name {
			return member, true
		}
	}
	return ParallelMemberConfig{}, false
}

// processStatusFromNode converts internal DAG progress into the public process status vocabulary.
func processStatusFromNode(node DAGNodeState, hasNode bool, sessionID string) string {
	if !hasNode {
		if sessionID != "" {
			return statusRunning
		}
		return ""
	}
	normalized := normalizeDAGNodeStatus(node.Status)
	switch normalized {
	case statusDone:
		return "completed"
	case statusRunning:
		return statusRunning
	case statusFailed:
		return statusFailed
	default:
		if node.Status != "" {
			return node.Status
		}
		if sessionID != "" {
			return statusRunning
		}
		return ""
	}
}

// normalizeDAGNodeStatus maps scheduler node states into run status groups.
func normalizeDAGNodeStatus(status string) string {
	switch normalizeRunStatus(status) {
	case runStatus(statusDone):
		return statusDone
	case runStatus(statusRunning):
		return statusRunning
	case runStatus(statusFailed):
		return statusFailed
	default:
		return status
	}
}

// fileExists reports whether a path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// newRunID produces a sortable run id.
func newRunID() string {
	return time.Now().UTC().Format("20060102T150405.000000000Z")
}
