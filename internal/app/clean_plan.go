// Package app builds and applies oz flow clean plans without mixing scan and delete work.
package app

import (
	"os"
	"runtime"
)

const (
	cleanActionDelete  = "delete"
	cleanActionProtect = "protect"
)

// CleanPlan records the runtime paths oz flow clean would delete or protect.
type CleanPlan struct {
	Repo              string                 `json:"repo"`
	DryRun            bool                   `json:"dry_run"`
	Runs              []CleanRunPlanItem     `json:"runs"`
	Batches           []CleanBatchPlanItem   `json:"batches"`
	AgentSessions     []CleanSessionPlanItem `json:"agent_sessions,omitempty"`
	CleanableSessions map[string]bool        `json:"-"`
	ProtectedSessions map[string]bool        `json:"-"`
}

// CleanRunPlanItem describes one run directory decision in a clean plan.
type CleanRunPlanItem struct {
	RunID  string `json:"run_id"`
	Path   string `json:"path"`
	Action string `json:"action"`
	Reason string `json:"reason"`
}

// CleanBatchPlanItem describes one batch directory decision in a clean plan.
type CleanBatchPlanItem struct {
	BatchID string   `json:"batch_id"`
	Path    string   `json:"path"`
	Action  string   `json:"action"`
	Reason  string   `json:"reason"`
	RunIDs  []string `json:"run_ids,omitempty"`
}

// CleanSessionPlanItem describes one external agent session decision in a clean plan.
type CleanSessionPlanItem struct {
	SessionID string `json:"session_id"`
	Action    string `json:"action"`
	Reason    string `json:"reason"`
}

// BuildCleanPlan scans repository runtime state and records decisions without deleting files.
func BuildCleanPlan(repo string, options CleanOptions) (CleanPlan, error) {
	plan := CleanPlan{
		Repo:              repo,
		CleanableSessions: map[string]bool{},
		ProtectedSessions: map[string]bool{},
	}
	cleanedRunIDs := map[string]bool{}
	protectedRunIDs := buildProtectedRunIDs(repo)

	runRoot, err := runsRoot(repo)
	if err != nil {
		return plan, err
	}
	runEntries, err := os.ReadDir(runRoot)
	if os.IsNotExist(err) {
		runEntries = nil
	} else if err != nil {
		return plan, err
	}
	for _, entry := range runEntries {
		if !entry.IsDir() {
			continue
		}
		runID := entry.Name()
		state, err := loadCleanRunState(repo, runID)
		if err != nil {
			if protectedRunIDs[runID] {
				plan.addProtectedRun(repo, runID, "corrupt run protected by active batch")
				continue
			}
			plan.addDeletedRun(repo, runID, "corrupt_or_missing_state")
			cleanedRunIDs[runID] = true
			continue
		}
		switch {
		case state.Status == statusDone || state.Status == statusArchived:
			collectAgentSessions(state, plan.ProtectedSessions)
			plan.addProtectedRun(repo, runID, "completed_history")
		case state.Status == statusRunning:
			collectAgentSessions(state, plan.ProtectedSessions)
			status, lockErr := lockFileStatus(repo, runID, runtime.GOOS)
			if lockErr == nil && status == lockStatusActive {
				plan.addProtectedRun(repo, runID, "active_lock")
				continue
			}
			plan.addProtectedRun(repo, runID, "running")
		case !cleanableRunStatuses[state.Status]:
			collectAgentSessions(state, plan.ProtectedSessions)
			plan.addProtectedRun(repo, runID, "unknown_or_retained_status")
		default:
			status, lockErr := lockFileStatus(repo, runID, runtime.GOOS)
			if lockErr == nil && status == lockStatusActive {
				collectAgentSessions(state, plan.ProtectedSessions)
				plan.addProtectedRun(repo, runID, "active_lock")
				continue
			}
			if protectedRunIDs[runID] {
				collectAgentSessions(state, plan.ProtectedSessions)
				plan.addProtectedRun(repo, runID, "batch_has_active_referenced_run")
				continue
			}
			collectAgentSessions(state, plan.CleanableSessions)
			plan.addDeletedRun(repo, runID, "cleanable_status:"+state.Status)
			cleanedRunIDs[runID] = true
		}
	}

	batchRoot, err := batchesRoot(repo)
	if err != nil {
		return plan, err
	}
	batchEntries, err := os.ReadDir(batchRoot)
	if os.IsNotExist(err) {
		batchEntries = nil
	} else if err != nil {
		return plan, err
	}
	for _, entry := range batchEntries {
		if !entry.IsDir() {
			continue
		}
		batchID := entry.Name()
		batch, err := loadCleanBatchState(repo, batchID)
		if err != nil {
			plan.addDeletedBatch(repo, batchID, nil, "corrupt_or_missing_state")
			continue
		}
		if !cleanableBatchStatuses[batch.Status] {
			plan.addProtectedBatch(repo, batchID, batchReferencedRunIDs(batch), "retained_status")
			continue
		}
		refdRunIDs := batchReferencedRunIDs(batch)
		if batchReferencesActiveRun(repo, refdRunIDs, cleanedRunIDs) {
			plan.addProtectedBatch(repo, batchID, refdRunIDs, "active_referenced_run")
			continue
		}
		for _, rid := range refdRunIDs {
			if cleanedRunIDs[rid] {
				continue
			}
			runState, err := loadCleanRunState(repo, rid)
			if err == nil && !cleanableRunStatuses[runState.Status] {
				collectAgentSessions(runState, plan.ProtectedSessions)
				continue
			}
			if _, statErr := os.Stat(runDir(repo, rid)); statErr == nil {
				if err == nil {
					collectAgentSessions(runState, plan.CleanableSessions)
				}
				plan.addDeletedRun(repo, rid, "cleanable_batch_reference")
			}
			cleanedRunIDs[rid] = true
		}
		plan.addDeletedBatch(repo, batchID, refdRunIDs, "cleanable_status:"+batch.Status)
	}
	if options.CleanAgentSessions {
		plan.addSessionItems()
	}
	return plan, nil
}

// ApplyCleanPlan deletes runtime paths selected by BuildCleanPlan and returns actual counts.
func ApplyCleanPlan(plan CleanPlan, options CleanOptions) (CleanResult, error) {
	var result CleanResult
	for _, item := range plan.Runs {
		if item.Action == cleanActionProtect && item.Reason == "active_lock" {
			result.SkippedRunning++
		}
		if item.Action != cleanActionDelete {
			continue
		}
		if err := os.RemoveAll(item.Path); err != nil {
			return result, err
		}
		result.CleanedRuns++
	}
	for _, item := range plan.Batches {
		if item.Action == cleanActionProtect && item.Reason == "active_referenced_run" {
			result.SkippedRunning++
		}
		if item.Action != cleanActionDelete {
			continue
		}
		if err := os.RemoveAll(item.Path); err != nil {
			return result, err
		}
		result.CleanedBatches++
	}
	if options.CleanAgentSessions {
		result.CleanedAgentRecords = cleanAgentSessionRecords(plan.CleanableSessions, plan.ProtectedSessions)
	}
	return result, nil
}

// Summary returns the counts a dry-run would apply without deleting anything.
func (plan CleanPlan) Summary(options CleanOptions) CleanResult {
	var result CleanResult
	for _, item := range plan.Runs {
		if item.Action == cleanActionDelete {
			result.CleanedRuns++
		}
		if item.Action == cleanActionProtect && item.Reason == "active_lock" {
			result.SkippedRunning++
		}
	}
	for _, item := range plan.Batches {
		if item.Action == cleanActionDelete {
			result.CleanedBatches++
		}
		if item.Action == cleanActionProtect && item.Reason == "active_referenced_run" {
			result.SkippedRunning++
		}
	}
	if options.CleanAgentSessions {
		for _, item := range plan.AgentSessions {
			if item.Action == cleanActionDelete {
				result.CleanedAgentRecords++
			}
		}
	}
	return result
}

// addDeletedRun appends a delete decision for a run directory.
func (plan *CleanPlan) addDeletedRun(repo, runID, reason string) {
	plan.Runs = append(plan.Runs, CleanRunPlanItem{RunID: runID, Path: runDir(repo, runID), Action: cleanActionDelete, Reason: reason})
}

// addProtectedRun appends a protect decision for a run directory.
func (plan *CleanPlan) addProtectedRun(repo, runID, reason string) {
	plan.Runs = append(plan.Runs, CleanRunPlanItem{RunID: runID, Path: runDir(repo, runID), Action: cleanActionProtect, Reason: reason})
}

// addDeletedBatch appends a delete decision for a batch directory.
func (plan *CleanPlan) addDeletedBatch(repo, batchID string, runIDs []string, reason string) {
	plan.Batches = append(plan.Batches, CleanBatchPlanItem{BatchID: batchID, Path: batchDir(repo, batchID), Action: cleanActionDelete, Reason: reason, RunIDs: runIDs})
}

// addProtectedBatch appends a protect decision for a batch directory.
func (plan *CleanPlan) addProtectedBatch(repo, batchID string, runIDs []string, reason string) {
	plan.Batches = append(plan.Batches, CleanBatchPlanItem{BatchID: batchID, Path: batchDir(repo, batchID), Action: cleanActionProtect, Reason: reason, RunIDs: runIDs})
}

// addSessionItems records external session decisions for JSON preview output.
func (plan *CleanPlan) addSessionItems() {
	for sessionID := range plan.CleanableSessions {
		if plan.ProtectedSessions[sessionID] {
			plan.AgentSessions = append(plan.AgentSessions, CleanSessionPlanItem{SessionID: sessionID, Action: cleanActionProtect, Reason: "referenced_by_retained_run"})
			continue
		}
		plan.AgentSessions = append(plan.AgentSessions, CleanSessionPlanItem{SessionID: sessionID, Action: cleanActionDelete, Reason: "only_referenced_by_deleted_runs"})
	}
	for sessionID := range plan.ProtectedSessions {
		if !plan.CleanableSessions[sessionID] {
			plan.AgentSessions = append(plan.AgentSessions, CleanSessionPlanItem{SessionID: sessionID, Action: cleanActionProtect, Reason: "referenced_by_retained_run"})
		}
	}
}

// batchReferencesActiveRun reports whether a batch references a currently locked run.
func batchReferencesActiveRun(repo string, runIDs []string, cleanedRunIDs map[string]bool) bool {
	for _, rid := range runIDs {
		if cleanedRunIDs[rid] {
			continue
		}
		status, err := lockFileStatus(repo, rid, runtime.GOOS)
		if err == nil && status == lockStatusActive {
			return true
		}
	}
	return false
}
