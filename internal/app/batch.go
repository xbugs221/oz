// Package app persists and runs serial batches of independent oz changes.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	batchStatusRunning  = "running"
	batchStatusDone     = "done"
	batchStatusFailed   = "failed"
	batchStatusAborted  = "aborted"
	batchStatusArchived = "archived"
)

// BatchState is the durable queue state for a serial batch run.
type BatchState struct {
	BatchID      string              `json:"batch_id"`
	Status       string              `json:"status"`
	Changes      []string            `json:"changes"`
	CurrentIndex int                 `json:"current_index"`
	RunIDs       map[string]string   `json:"run_ids"`
	FailedChange string              `json:"failed_change,omitempty"`
	FailedRunID  string              `json:"failed_run_id,omitempty"`
	Error        string              `json:"error"`
	Worker       *WorkerRuntimeState `json:"worker,omitempty"`
}

// withBatchState locks a batch with a cross-process file lock, reads its state,
// calls fn, and writes back.
func withBatchState(repo, batchID string, fn func(*BatchState) error) error {
	lockPath := batchDir(repo, batchID) + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return fmt.Errorf("创建 batch 目录失败：%w", err)
	}
	unlock, err := flockLock(lockPath)
	if err != nil {
		return fmt.Errorf("获取 batch 锁失败：%w", err)
	}
	defer unlock()
	batch, err := loadBatchState(repo, batchID)
	if err != nil {
		return err
	}
	if err := fn(&batch); err != nil {
		return err
	}
	return saveBatchState(repo, batch)
}

// SubmitBatch creates a batch state and starts one detached batch worker.
func (e *Engine) SubmitBatch(ctx context.Context, changes []Change) error {
	_ = ctx
	sorted := SortChangesByNumericPrefix(changes)
	if _, _, err := gitSnapshot(e.Repo); err != nil {
		return err
	}
	batch := BatchState{
		BatchID:      newRunID(),
		Status:       batchStatusRunning,
		Changes:      make([]string, 0, len(sorted)),
		CurrentIndex: 0,
		RunIDs:       map[string]string{},
	}
	for _, change := range sorted {
		batch.Changes = append(batch.Changes, change.Name)
	}
	if err := saveBatchState(e.Repo, batch); err != nil {
		return err
	}
	if err := startDetachedBatchCommand(e.Repo, batch.BatchID); err != nil {
		return err
	}
	if e.Output != nil {
		fmt.Fprintf(e.Output, "已提交批量任务 %s (1/%d)\n", batch.BatchID, len(batch.Changes))
	}
	return nil
}

// AppendBatchChanges appends active changes to a running batch, skipping duplicates.
// It validates that each change is currently active by querying oz.
func AppendBatchChanges(repo, batchID string, changes []Change) (added []string, skipped []string, err error) {
	activeChanges, err := ListChanges(repo)
	if err != nil {
		return nil, nil, fmt.Errorf("读取 active changes 失败：%w", err)
	}
	activeMap := map[string]bool{}
	for _, c := range activeChanges {
		activeMap[c.Name] = true
	}
	err = withBatchState(repo, batchID, func(batch *BatchState) error {
		if batch.Status != batchStatusRunning {
			switch batch.Status {
			case batchStatusDone:
				return fmt.Errorf("批量任务已完成，请发起新的批量任务")
			case batchStatusFailed:
				return fmt.Errorf("批量任务已失败，请先处理失败项或重新发起")
			case batchStatusAborted:
				return fmt.Errorf("批量任务已中止，不能追加")
			default:
				return fmt.Errorf("批量任务状态 %s 不支持追加", batch.Status)
			}
		}
		existing := map[string]bool{}
		for _, name := range batch.Changes {
			existing[name] = true
		}
		var newChanges []Change
		for _, c := range changes {
			if existing[c.Name] {
				skipped = append(skipped, c.Name)
				continue
			}
			if !activeMap[c.Name] {
				return fmt.Errorf("提案 %s 不存在或已归档", c.Name)
			}
			newChanges = append(newChanges, c)
		}
		if len(newChanges) == 0 {
			return nil
		}
		sorted := SortChangesByNumericPrefix(newChanges)
		for _, c := range sorted {
			batch.Changes = append(batch.Changes, c.Name)
			added = append(added, c.Name)
		}
		return nil
	})
	return added, skipped, err
}

// FilterChangesNotInBatch returns active changes that are not already queued.
func FilterChangesNotInBatch(changes []Change, batch BatchState) []Change {
	existing := map[string]bool{}
	for _, name := range batch.Changes {
		existing[name] = true
	}
	filtered := make([]Change, 0, len(changes))
	for _, change := range changes {
		if !existing[change.Name] {
			filtered = append(filtered, change)
		}
	}
	return filtered
}

// RunBatch advances a batch until all changes finish or one run stops.
func (e *Engine) RunBatch(ctx context.Context, batchID string) error {
	batch, err := loadBatchState(e.Repo, batchID)
	if err != nil {
		return err
	}
	if batch.Status != batchStatusRunning {
		return fmt.Errorf("批量任务 %s 状态 %s 不能直接继续，请使用 oz flow restart", batchID, batch.Status)
	}
	for {
		for batch.Status == batchStatusRunning && batch.CurrentIndex < len(batch.Changes) {
			changeName := batch.Changes[batch.CurrentIndex]
			runID := batch.RunIDs[changeName]
			if runID == "" {
				state, err := e.createRun(changeName)
				if err != nil {
					return e.failBatch(batch.BatchID, changeName, "", err)
				}
				state.BatchID = batch.BatchID
				state.BatchIndex = batch.CurrentIndex + 1
				state.BatchTotal = len(batch.Changes)
				if err := saveState(e.Repo, state); err != nil {
					return err
				}
				runID = state.RunID
				if err := withBatchState(e.Repo, batchID, func(b *BatchState) error {
					b.RunIDs[changeName] = runID
					return nil
				}); err != nil {
					return err
				}
				batch.RunIDs[changeName] = runID
			}
			state, err := loadState(e.Repo, runID)
			if err != nil {
				return e.failBatch(batch.BatchID, changeName, runID, err)
			}
			if state.Status == statusDone {
				if err := withBatchState(e.Repo, batchID, func(b *BatchState) error {
					b.CurrentIndex++
					return nil
				}); err != nil {
					return err
				}
				batch.CurrentIndex++
			} else if isBatchTerminalState(state) {
				return e.failBatch(batch.BatchID, changeName, runID, fmt.Errorf("run %s 已停止：%s/%s", runID, state.Status, state.Stage))
			} else {
				if err := e.resumeRun(ctx, runID, true, nil); err != nil {
					if isRunLockedError(err) {
						return err
					}
					latest, loadErr := loadState(e.Repo, runID)
					if loadErr == nil {
						latest = failedState(latest, err)
						_ = saveState(e.Repo, latest)
					}
					return e.failBatch(batch.BatchID, changeName, runID, err)
				}
			}
			// Reload batch state to pick up any appended changes.
			batch, err = loadBatchState(e.Repo, batchID)
			if err != nil {
				return err
			}
		}
		if batch.Status != batchStatusRunning {
			return nil
		}
		// Guard the final done transition inside the lock to avoid
		// skipping changes appended between the last reload and write.
		if err := withBatchState(e.Repo, batchID, func(b *BatchState) error {
			if b.CurrentIndex < len(b.Changes) {
				return nil
			}
			b.Status = batchStatusDone
			b.CurrentIndex = len(b.Changes)
			return nil
		}); err != nil {
			return err
		}
		batch, err = loadBatchState(e.Repo, batchID)
		if err != nil {
			return err
		}
		if batch.Status != batchStatusRunning || batch.CurrentIndex >= len(batch.Changes) {
			return nil
		}
	}
}

// AbortBatch marks a batch aborted and aborts its current run when present.
func AbortBatch(repo, batchID string) error {
	batch, err := loadBatchState(repo, batchID)
	if err != nil {
		return err
	}
	if batch.CurrentIndex < len(batch.Changes) {
		changeName := batch.Changes[batch.CurrentIndex]
		if runID := batch.RunIDs[changeName]; runID != "" {
			if err := AbortRun(repo, runID); err != nil {
				return err
			}
		}
	}
	batch.Status = batchStatusAborted
	batch.Error = "用户已中止"
	return saveBatchState(repo, batch)
}

// FindUnfinishedBatch returns the newest batch whose state is still running.
func FindUnfinishedBatch(repo string) (string, error) {
	root, err := batchesRoot(repo)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if !entry.IsDir() {
			continue
		}
		batch, err := loadBatchState(repo, entry.Name())
		if err == nil && batch.Status == batchStatusRunning {
			return entry.Name(), nil
		}
	}
	return "", nil
}

// FindStartupBatches returns the newest running batch and stopped batches for startup prompts.
func FindStartupBatches(repo string) (string, []BatchState, error) {
	root, err := batchesRoot(repo)
	if err != nil {
		return "", nil, err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	var running string
	var stopped []BatchState
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if !entry.IsDir() {
			continue
		}
		batch, err := loadBatchState(repo, entry.Name())
		if err != nil {
			continue
		}
		if isStoppedBatchState(batch) {
			stopped = append(stopped, batch)
			continue
		}
		if running == "" && batch.Status == batchStatusRunning {
			running = entry.Name()
		}
	}
	return running, stopped, nil
}

// isStoppedBatchState reports terminal batch states shown as stopped, not resumable.
func isStoppedBatchState(batch BatchState) bool {
	return batch.Status == batchStatusFailed || batch.Status == batchStatusAborted
}

func (e *Engine) failBatch(batchID, changeName, runID string, err error) error {
	if saveErr := withBatchState(e.Repo, batchID, func(batch *BatchState) error {
		batch.Status = batchStatusFailed
		batch.FailedChange = changeName
		batch.FailedRunID = runID
		if err != nil {
			batch.Error = err.Error()
		}
		return nil
	}); saveErr != nil {
		return saveErr
	}
	return err
}

func isBatchTerminalState(state State) bool {
	switch state.Status {
	case statusFailed, statusBlocked, statusValidationBlocked, statusAcceptanceContractBlocked, statusAborted, statusInterrupted, "aborted":
		return true
	}
	return state.Stage == statusBlocked || state.Stage == statusValidationBlocked || state.Stage == statusAcceptanceContractBlocked
}

func saveBatchState(repo string, batch BatchState) error {
	if batch.RunIDs == nil {
		batch.RunIDs = map[string]string{}
	}
	root := batchDir(repo, batch.BatchID)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(batch, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "state.json"), append(data, '\n'), 0o644)
}

func loadBatchState(repo, batchID string) (BatchState, error) {
	data, err := os.ReadFile(filepath.Join(batchDir(repo, batchID), "state.json"))
	if err != nil {
		return BatchState{}, err
	}
	var batch BatchState
	if err := json.Unmarshal(data, &batch); err != nil {
		return BatchState{}, err
	}
	if batch.RunIDs == nil {
		batch.RunIDs = map[string]string{}
	}
	return batch, nil
}

// FindLatestBatch returns the newest batch state regardless of status.
func FindLatestBatch(repo string) (*BatchState, error) {
	root, err := batchesRoot(repo)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if !entry.IsDir() {
			continue
		}
		batch, err := loadBatchState(repo, entry.Name())
		if err != nil {
			continue
		}
		return &batch, nil
	}
	return nil, nil
}

// batchStatusLines formats a batch overview with change queue and run states.
func batchStatusLines(repo string, batch *BatchState, batchAlias string, _ []StatusRef) []string {
	var lines []string
	if batchAlias == "" {
		batchAlias = batch.BatchID
	}
	if batch.Status == batchStatusRunning {
		total := len(batch.Changes)
		current := batch.CurrentIndex + 1
		if total == 0 {
			current = 0
		} else if current > total {
			current = total
		}
		lines = append(lines, fmt.Sprintf("批量任务 %s %s %d/%d", batchAlias, batch.Status, current, total))
	}

	for _, changeName := range batch.Changes {
		runID := batch.RunIDs[changeName]
		if runID != "" {
			if state, err := loadState(repo, runID); err == nil {
				runRefs, _ := ListRunRefs(repo)
				runAlias := RunAliasForID(runRefs, runID)
				view := buildHumanStatusView(repo, state, runAlias, "→")
				lines = append(lines, statusHeaderText(changeName, view))
				for _, line := range compactStatusLines(view) {
					lines = append(lines, fmt.Sprintf("  %s", line))
				}
				if batch.Status == batchStatusRunning && isStaleRunningRun(repo, state) {
					lines = append(lines, fmt.Sprintf("  提示: 当前 run 的 lock 已失效，可运行 oz flow restart -%s 重试当前批量阶段", batchAlias))
				}
				continue
			}
		}
		lines = append(lines, fmt.Sprintf("- %s", changeName))
	}
	if batch.Status == batchStatusFailed || batch.Status == batchStatusAborted {
		lines = append(lines, batchFailureLines(repo, *batch, batchAlias)...)
	}

	return lines
}

// batchFailureLines returns human-readable failure reason and recovery/cleanup hints.
func batchFailureLines(repo string, batch BatchState, batchAlias string) []string {
	var lines []string
	failedChange := batch.FailedChange
	if failedChange == "" && batch.CurrentIndex < len(batch.Changes) {
		failedChange = batch.Changes[batch.CurrentIndex]
	}
	runID := batch.FailedRunID
	if runID == "" && failedChange != "" {
		runID = batch.RunIDs[failedChange]
	}

	summary := humanBatchFailureSummary(repo, batch, failedChange, runID)
	lines = append(lines, fmt.Sprintf("  错误: %s", summary))

	// Check if this is recoverable via restart.
	if batch.Status == batchStatusFailed {
		if isBatchRestartRecoverable(repo, batch) {
			lines = append(lines, fmt.Sprintf("  提示: 可运行 oz flow restart -%s 删除失败记录并继续该批量任务", batchAlias))
			lines = append(lines, fmt.Sprintf("  归档: 可运行 %s 归档失败记录后开启新工作流", archiveCommandForAlias(batchAlias)))
		} else {
			lines = append(lines, fmt.Sprintf("  归档: 可运行 %s 归档失败记录后开启新工作流", archiveCommandForAlias(batchAlias)))
			lines = append(lines, "  清理: 可运行 oz flow clean 清理当前项目失败或异常运行态")
			lines = append(lines, "        该操作仅删除 oz flow 历史记录，不回滚代码改动")
		}
	} else if batch.Status == batchStatusAborted {
		lines = append(lines, "  清理: 可运行 oz flow clean 清理当前项目失败或异常运行态")
		lines = append(lines, "        该操作仅删除 oz flow 历史记录，不回滚代码改动")
	}
	return lines
}

// humanBatchFailureSummary builds a human-readable failure summary for a stopped batch.
func humanBatchFailureSummary(repo string, batch BatchState, changeName, runID string) string {
	if batch.Status == batchStatusAborted {
		if reason := batch.Error; reason != "" {
			return reason
		}
		return "用户已中止"
	}
	if runID != "" {
		if state, err := loadState(repo, runID); err == nil {
			return humanRunFailureSummary(state, changeName)
		}
		prefix := ""
		if changeName != "" {
			prefix = changeName + " 的"
		}
		return prefix + "工作流记录缺失，无法自动确认恢复方式"
	}
	if batch.Error != "" {
		return sanitizeErrorForHuman(batch.Error)
	}
	return batch.Status
}

// isBatchRestartRecoverable reports whether a failed batch can be recovered via restart.
func isBatchRestartRecoverable(repo string, batch BatchState) bool {
	if batch.Status != batchStatusFailed {
		return false
	}
	runID := batch.FailedRunID
	if runID == "" && batch.CurrentIndex < len(batch.Changes) {
		runID = batch.RunIDs[batch.Changes[batch.CurrentIndex]]
	}
	if runID == "" {
		return true
	}
	state, err := loadState(repo, runID)
	if err != nil {
		return false
	}
	if runFailedAfterSuccessfulArchive(repo, state) {
		return false
	}
	return isRestartableRunCandidate(repo, state)
}

// runFailedAfterSuccessfulArchive reports the stale failure shape caused by post-archive gates.
func runFailedAfterSuccessfulArchive(repo string, state State) bool {
	return state.Status == statusFailed &&
		state.Stage == "archive" &&
		archiveExists(repo, state.ChangeName) &&
		!fileExists(acceptancePath(repo, state.ChangeName))
}

// humanBatchStatusError returns the public error summary for batch status output.
// Deprecated: use humanBatchFailureSummary for richer context.
func humanBatchStatusError(repo string, batch BatchState) string {
	if batch.Status == batchStatusFailed {
		return stoppedBatchReason(repo, batch)
	}
	return batch.Error
}

// batchCleanupPath returns the human-readable filesystem path for batch state removal.
func batchCleanupPath(repo, batchID string) string {
	return batchDir(repo, batchID)
}

func startDetachedBatchResumeCommand(repo, batchID string) error {
	exe, err := currentExecutable()
	if err != nil {
		return fmt.Errorf("解析 oz flow 可执行文件失败：%w", err)
	}
	cmd := exec.Command(exe, flowWorkerCommandArgs("batch", "--batch-id", batchID, "--json")...)
	cmd.Dir = repo
	configureDetachedCommand(cmd)
	return startDetachedWorkerCommand(cmd, batchWorkerLogPath(repo, batchID))
}
