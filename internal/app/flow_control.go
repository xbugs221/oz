// Package app controls operator-level oz flow batch lifecycle commands.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const loopPendingStartGrace = 2 * time.Minute

// dispatchStopCommand stops the current running batch workflow.
func dispatchStopCommand(args []string, stdout io.Writer, repo string) error {
	if len(args) != 1 {
		return fmt.Errorf("用法：oz flow stop")
	}
	batchID, err := FindUnfinishedBatch(repo)
	if err != nil {
		return err
	}
	if batchID == "" {
		fmt.Fprintln(stdout, "当前没有正在进行的批量工作流")
		return nil
	}
	if err := AbortBatch(repo, batchID); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "已停止批量工作流 %s\n", batchID)
	return nil
}

// dispatchArchiveCommand archives the newest failed workflow runtime record.
func dispatchArchiveCommand(args []string, stdout io.Writer, repo string) error {
	kind, ref, err := ResolveArchiveTarget(repo, args[1:])
	if err != nil {
		return err
	}
	switch kind {
	case "batch":
		if _, err := ArchiveFailedBatch(repo, ref.ID); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "已归档失败批量工作流 %s，可运行 oz flow run 开启新工作流\n", ref.Alias)
		return nil
	case "run":
		if err := ArchiveFailedRun(repo, ref.ID, "archived failed workflow"); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "已归档失败工作流 %s，可运行 oz flow run 开启新工作流\n", ref.Alias)
		return nil
	default:
		return fmt.Errorf("没有可归档失败工作流")
	}
}

// dispatchLoopCommand monitors batch progress and starts continuation batches after failures.
func dispatchLoopCommand(ctx context.Context, args []string, stdout io.Writer, repo string, engine *Engine) error {
	if len(args) == 3 && args[1] == "--worker" && args[2] == "--json" {
		return runBatchLoopWorker(ctx, stdout, repo, engine, time.Minute)
	}
	if len(args) != 1 {
		return fmt.Errorf("用法：oz flow loop")
	}
	active, err := loopWorkerActive(repo)
	if err != nil {
		return err
	}
	if active {
		fmt.Fprintln(stdout, "后台 loop 监控已在运行")
		return nil
	}
	if err := startDetachedLoopCommand(repo); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "已启动后台 loop 监控")
	return nil
}

// ResolveArchiveTarget maps archive arguments to the newest archivable batch or run.
func ResolveArchiveTarget(repo string, args []string) (kind string, ref StatusRef, err error) {
	if len(args) == 0 {
		batches, err := ListBatchRefs(repo)
		if err != nil {
			return "", StatusRef{}, err
		}
		for _, batchRef := range batches {
			batch, err := loadBatchState(repo, batchRef.ID)
			if err == nil && isArchivableBatch(batch) {
				return "batch", batchRef, nil
			}
		}
		runs, err := ListRunRefs(repo)
		if err != nil {
			return "", StatusRef{}, err
		}
		for _, runRef := range runs {
			state, err := loadState(repo, runRef.ID)
			if err == nil && state.BatchID == "" && isArchivableRun(state) {
				return "run", runRef, nil
			}
		}
		return "", StatusRef{}, fmt.Errorf("没有可归档失败工作流")
	}
	if len(args) != 1 {
		return "", StatusRef{}, fmt.Errorf("用法：oz flow archive [-bN|-wN]")
	}
	switch {
	case strings.HasPrefix(args[0], "-b"):
		ref, err := resolveIndexedRef(repo, args[0], "-b", ListBatchRefs)
		return "batch", ref, err
	case strings.HasPrefix(args[0], "-w"):
		ref, err := resolveIndexedRef(repo, args[0], "-w", ListRunRefs)
		return "run", ref, err
	default:
		return "", StatusRef{}, fmt.Errorf("用法：oz flow archive [-bN|-wN]")
	}
}

// ArchiveFailedBatch marks a failed batch as archived and preserves its failed run record.
func ArchiveFailedBatch(repo, batchID string) (BatchState, error) {
	batch, err := loadBatchState(repo, batchID)
	if err != nil {
		return BatchState{}, err
	}
	if !isArchivableBatch(batch) {
		return BatchState{}, fmt.Errorf("批量工作流 %s 状态 %s 不能归档", batchID, batch.Status)
	}
	if runID := failedRunIDForBatch(batch); runID != "" {
		if err := ArchiveFailedRun(repo, runID, "archived with failed batch "+batchID); err != nil && !isAlreadyArchivedRunError(err) {
			return BatchState{}, err
		}
	}
	batch.Status = batchStatusArchived
	if batch.Error == "" {
		batch.Error = "已归档失败批量工作流"
	}
	if err := saveBatchState(repo, batch); err != nil {
		return BatchState{}, err
	}
	return batch, nil
}

// ArchiveFailedRun marks a failed standalone run as archived without deleting its evidence.
func ArchiveFailedRun(repo, runID, reason string) error {
	state, err := loadState(repo, runID)
	if err != nil {
		return err
	}
	if state.Status == statusArchived {
		return alreadyArchivedRunError{runID: runID}
	}
	if !isArchivableRun(state) {
		return fmt.Errorf("工作流 %s 状态 %s 不能归档", runID, state.Status)
	}
	if err := clearArchiveLock(repo, runID); err != nil {
		return err
	}
	state.Status = statusArchived
	if reason != "" {
		state.Error = reason
	}
	if err := saveState(repo, state); err != nil {
		return err
	}
	return nil
}

// runBatchLoop polls batch state until the batch succeeds or the operator stops it.
func runBatchLoop(ctx context.Context, stdout io.Writer, repo string, engine *Engine, interval time.Duration) error {
	if interval <= 0 {
		interval = time.Minute
	}
	for {
		done, err := runBatchLoopTick(ctx, stdout, repo, engine)
		if err != nil || done {
			return err
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// runBatchLoopWorker holds the repo loop lock while the detached monitor is active.
func runBatchLoopWorker(ctx context.Context, stdout io.Writer, repo string, engine *Engine, interval time.Duration) error {
	unlock, acquired, err := acquireLoopWorkerLock(repo)
	if err != nil || !acquired {
		return err
	}
	defer unlock()
	return withLoopWorkerLifecycle(repo, func() error {
		return runBatchLoop(ctx, stdout, repo, engine, interval)
	})
}

// runBatchLoopTick performs one monitor cycle and returns true when monitoring is complete.
func runBatchLoopTick(ctx context.Context, stdout io.Writer, repo string, engine *Engine) (bool, error) {
	batchID, err := FindUnfinishedBatch(repo)
	if err != nil {
		return false, err
	}
	if batchID != "" {
		batch, err := loadBatchState(repo, batchID)
		if err != nil {
			return false, err
		}
		decision := assessBatchForLoop(repo, batch)
		switch decision.Status {
		case loopDecisionRunning:
			return false, nil
		case loopDecisionDone:
			fmt.Fprintf(stdout, "批量工作流 %s 已完成\n", batch.BatchID)
			return true, nil
		case loopDecisionStopped:
			fmt.Fprintf(stdout, "批量工作流 %s 已停止，loop 退出\n", batch.BatchID)
			return true, nil
		case loopDecisionFailed:
			return archiveAndSubmitContinuation(ctx, stdout, repo, engine, batch, decision.Reason)
		}
	}
	latest, err := FindLatestBatch(repo)
	if err != nil {
		return false, err
	}
	if latest != nil {
		decision := assessBatchForLoop(repo, *latest)
		switch decision.Status {
		case loopDecisionDone:
			return submitActiveChangesOrFinishLoop(ctx, stdout, repo, engine, fmt.Sprintf("批量工作流 %s 已完成\n", latest.BatchID))
		case loopDecisionStopped:
			return submitActiveChangesOrFinishLoop(ctx, stdout, repo, engine, fmt.Sprintf("批量工作流 %s 已停止，loop 退出\n", latest.BatchID))
		case loopDecisionFailed:
			return archiveAndSubmitContinuation(ctx, stdout, repo, engine, *latest, decision.Reason)
		}
	}
	return submitActiveChangesForLoop(ctx, stdout, repo, engine)
}

// archiveAndSubmitContinuation archives a failed batch and submits its unfinished active changes.
func archiveAndSubmitContinuation(ctx context.Context, stdout io.Writer, repo string, engine *Engine, batch BatchState, reason string) (bool, error) {
	changes, err := continuationChanges(repo, batch)
	if err != nil {
		return false, err
	}
	if err := markStaleRunForLoop(repo, failedRunIDForBatch(batch), reason); err != nil {
		return false, err
	}
	if batch.Status == batchStatusRunning {
		if err := markBatchFailedForLoop(repo, batch, reason); err != nil {
			return false, err
		}
		latest, err := loadBatchState(repo, batch.BatchID)
		if err != nil {
			return false, err
		}
		batch = latest
	}
	if _, err := ArchiveFailedBatch(repo, batch.BatchID); err != nil {
		return false, err
	}
	if reason == "" {
		reason = "失败"
	}
	fmt.Fprintf(stdout, "已归档失败批量工作流 %s：%s\n", batch.BatchID, reason)
	if len(changes) == 0 {
		fmt.Fprintln(stdout, "没有可继续的 active 变更提案，loop 退出")
		return true, nil
	}
	return false, engine.SubmitBatch(ctx, changes)
}

// submitActiveChangesForLoop starts a batch when loop is launched without an active one.
func submitActiveChangesForLoop(ctx context.Context, stdout io.Writer, repo string, engine *Engine) (bool, error) {
	return submitActiveChangesOrFinishLoop(ctx, stdout, repo, engine, "没有 active 变更提案，loop 退出\n")
}

// submitActiveChangesOrFinishLoop starts active proposals, otherwise prints the terminal message.
func submitActiveChangesOrFinishLoop(ctx context.Context, stdout io.Writer, repo string, engine *Engine, terminalMessage string) (bool, error) {
	changes, err := ListChanges(repo)
	if err != nil {
		return false, err
	}
	if len(changes) == 0 {
		fmt.Fprint(stdout, terminalMessage)
		return true, nil
	}
	if err := engine.SubmitBatch(ctx, changes); err != nil {
		return false, err
	}
	return false, nil
}

// startDetachedLoopWorkerCommand starts the long-running loop monitor outside the terminal.
func startDetachedLoopWorkerCommand(repo string) error {
	exe, err := currentExecutable()
	if err != nil {
		return fmt.Errorf("解析 oz flow 可执行文件失败：%w", err)
	}
	cmd := exec.Command(exe, flowWorkerCommandArgs("loop", "--worker", "--json")...)
	cmd.Dir = repo
	configureDetachedCommand(cmd)
	logPath, err := loopWorkerLogPath(repo)
	if err != nil {
		return err
	}
	return startDetachedWorkerCommand(cmd, logPath)
}

// loopWorkerActive reports whether a live background loop monitor already owns this repository.
func loopWorkerActive(repo string) (bool, error) {
	path, err := loopWorkerLockPath(repo)
	if err != nil {
		return false, err
	}
	status, err := lockInfoFileStatus(path, runtime.GOOS)
	if err != nil {
		return false, err
	}
	if status == lockStatusUnknown {
		return false, fmt.Errorf("loop lock 无法确认，请稍后重试或手动检查 %s", path)
	}
	return status == lockStatusActive, nil
}

// acquireLoopWorkerLock creates the repo-level lock for one detached loop monitor.
func acquireLoopWorkerLock(repo string) (func(), bool, error) {
	path, err := loopWorkerLockPath(repo)
	if err != nil {
		return nil, false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, false, err
	}
	status, err := lockInfoFileStatus(path, runtime.GOOS)
	if err != nil {
		return nil, false, err
	}
	switch status {
	case lockStatusActive:
		return nil, false, nil
	case lockStatusUnknown:
		return nil, false, fmt.Errorf("loop lock 无法确认，请稍后重试或手动检查 %s", path)
	case lockStatusStale:
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, false, err
		}
	}
	hostname, _ := os.Hostname()
	lock := LockInfo{
		PID:       os.Getpid(),
		Hostname:  hostname,
		RunID:     "loop",
		StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return nil, false, err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return nil, false, err
	}
	return func() { _ = os.Remove(path) }, true, nil
}

// loopWorkerLockPath returns the repository-scoped monitor lock file.
func loopWorkerLockPath(repo string) (string, error) {
	base, err := repoRuntimeDir(repo)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "loop.lock"), nil
}

type loopDecisionStatus string

const (
	loopDecisionRunning loopDecisionStatus = "running"
	loopDecisionDone    loopDecisionStatus = "done"
	loopDecisionFailed  loopDecisionStatus = "failed"
	loopDecisionStopped loopDecisionStatus = "stopped"
)

const staleRunWorkerHeartbeatReason = "当前 run worker heartbeat 已失效"

type loopDecision struct {
	Status loopDecisionStatus
	Reason string
}

// assessBatchForLoop decides whether a batch can still advance without intervention.
func assessBatchForLoop(repo string, batch BatchState) loopDecision {
	switch batch.Status {
	case batchStatusDone:
		return loopDecision{Status: loopDecisionDone}
	case batchStatusAborted:
		return loopDecision{Status: loopDecisionStopped}
	case batchStatusFailed:
		return loopDecision{Status: loopDecisionFailed, Reason: stoppedBatchReason(repo, batch)}
	case batchStatusArchived:
		return loopDecision{Status: loopDecisionStopped}
	}
	if batch.Status != batchStatusRunning {
		return loopDecision{Status: loopDecisionFailed, Reason: "未知批量状态 " + batch.Status}
	}
	now := time.Now()
	if batch.CurrentIndex >= len(batch.Changes) {
		if err := markBatchDone(repo, batch.BatchID); err != nil {
			return loopDecision{Status: loopDecisionFailed, Reason: err.Error()}
		}
		return loopDecision{Status: loopDecisionDone}
	}
	runID := restartBatchCurrentRunID(batch)
	if runID == "" {
		if pendingBatchStartExpired(repo, batch, now) {
			return loopDecision{Status: loopDecisionFailed, Reason: "批量 worker 未创建当前 run"}
		}
		return loopDecision{Status: loopDecisionRunning}
	}
	state, err := loadState(repo, runID)
	if err != nil {
		return loopDecision{Status: loopDecisionFailed, Reason: err.Error()}
	}
	if isBatchTerminalState(state) {
		return loopDecision{Status: loopDecisionFailed, Reason: humanRunFailureSummary(state, state.ChangeName)}
	}
	if stopped, reason := runningBatchRunCannotAdvance(repo, batch, state, now); stopped {
		return loopDecision{Status: loopDecisionFailed, Reason: reason}
	}
	return loopDecision{Status: loopDecisionRunning}
}

// continuationChanges keeps the unfinished active tail from the failed batch queue.
func continuationChanges(repo string, batch BatchState) ([]Change, error) {
	active, err := ListChanges(repo)
	if err != nil {
		return nil, err
	}
	activeSet := map[string]bool{}
	for _, change := range active {
		activeSet[change.Name] = true
	}
	start := batch.CurrentIndex
	if start < 0 {
		start = 0
	}
	if start > len(batch.Changes) {
		start = len(batch.Changes)
	}
	var changes []Change
	for _, name := range batch.Changes[start:] {
		if activeSet[name] {
			changes = append(changes, Change{Name: name, Path: changePath(repo, name)})
		}
	}
	return changes, nil
}

// markBatchDone fixes a fully advanced running batch whose worker exited before final save.
func markBatchDone(repo, batchID string) error {
	return withBatchState(repo, batchID, func(batch *BatchState) error {
		if batch.CurrentIndex >= len(batch.Changes) && batch.Status == batchStatusRunning {
			batch.Status = batchStatusDone
			batch.CurrentIndex = len(batch.Changes)
		}
		return nil
	})
}

// pendingBatchStartExpired reports a batch worker that never created its first run.
func pendingBatchStartExpired(repo string, batch BatchState, now time.Time) bool {
	info, err := os.Stat(filepath.Join(batchDir(repo, batch.BatchID), "state.json"))
	if err != nil {
		return false
	}
	return now.Sub(info.ModTime()) > loopPendingStartGrace
}

// runningRunCannotAdvance catches orphaned running state that has no live owner.
func runningRunCannotAdvance(repo string, state State, now time.Time) (bool, string) {
	if state.Status != statusRunning || state.RunID == "" {
		return false, ""
	}
	status, err := lockFileStatus(repo, state.RunID, runtime.GOOS)
	if err != nil {
		return false, ""
	}
	switch status {
	case lockStatusActive:
		if workerHeartbeatExpired(state.Worker, now) {
			return true, staleRunWorkerHeartbeatReason
		}
	case lockStatusStale:
		return true, "当前 run lock 已失效"
	case lockStatusNone:
		info, err := os.Stat(filepath.Join(runDir(repo, state.RunID), "state.json"))
		if err == nil && now.Sub(info.ModTime()) > loopPendingStartGrace {
			return true, "当前 run 没有活动 lock"
		}
	}
	return false, ""
}

// runningBatchRunCannotAdvance keeps a batch-owned run alive while the batch
// worker is still heartbeating, even if the run heartbeat is temporarily stale.
func runningBatchRunCannotAdvance(repo string, batch BatchState, state State, now time.Time) (bool, string) {
	stopped, reason := runningRunCannotAdvance(repo, state, now)
	if !stopped || reason != staleRunWorkerHeartbeatReason {
		return stopped, reason
	}
	if batchWorkerOwnsCurrentRun(batch, state, now) {
		return false, ""
	}
	return stopped, reason
}

// batchWorkerOwnsCurrentRun reports whether the running batch worker still owns
// the current run and can continue refreshing it across stage boundaries.
func batchWorkerOwnsCurrentRun(batch BatchState, state State, now time.Time) bool {
	if batch.Status != batchStatusRunning || state.BatchID == "" || state.BatchID != batch.BatchID {
		return false
	}
	if batch.CurrentIndex < 0 || batch.CurrentIndex >= len(batch.Changes) {
		return false
	}
	if batch.RunIDs[batch.Changes[batch.CurrentIndex]] != state.RunID {
		return false
	}
	return workerHeartbeatFresh(batch.Worker, now)
}

// markBatchFailedForLoop records a monitor-detected failure before archival.
func markBatchFailedForLoop(repo string, batch BatchState, reason string) error {
	return withBatchState(repo, batch.BatchID, func(current *BatchState) error {
		if current.Status != batchStatusRunning {
			return nil
		}
		current.Status = batchStatusFailed
		current.FailedChange = ""
		if current.CurrentIndex >= 0 && current.CurrentIndex < len(current.Changes) {
			current.FailedChange = current.Changes[current.CurrentIndex]
			current.FailedRunID = current.RunIDs[current.FailedChange]
		}
		if reason != "" {
			current.Error = reason
		}
		return nil
	})
}

// markStaleRunForLoop records a monitor-confirmed orphan before batch archival.
func markStaleRunForLoop(repo, runID, reason string) error {
	if runID == "" {
		return nil
	}
	state, err := loadState(repo, runID)
	if err != nil {
		return err
	}
	if state.Status != statusRunning {
		return nil
	}
	status, err := lockFileStatus(repo, runID, runtime.GOOS)
	if err != nil {
		return err
	}
	if status != lockStatusStale {
		return fmt.Errorf("run %s 的 lock 状态已变化，拒绝回收", runID)
	}
	return mergeState(repo, runID, func(state *State) {
		if state.Status == statusRunning {
			state.Status = statusStale
			state.Error = reason
		}
	})
}

// failedRunIDForBatch returns the run that caused a failed batch to stop.
func failedRunIDForBatch(batch BatchState) string {
	if batch.FailedRunID != "" {
		return batch.FailedRunID
	}
	if batch.CurrentIndex >= 0 && batch.CurrentIndex < len(batch.Changes) {
		return batch.RunIDs[batch.Changes[batch.CurrentIndex]]
	}
	return ""
}

// isArchivableBatch reports whether archive can hide a failed batch from active prompts.
func isArchivableBatch(batch BatchState) bool {
	return batch.Status == batchStatusFailed
}

// isArchivableRun reports whether a run stopped because the workflow could not proceed.
func isArchivableRun(state State) bool {
	switch state.Status {
	case statusFailed, statusStale, statusInterrupted, statusBlocked, statusValidationBlocked, statusAcceptanceContractBlocked:
		return true
	default:
		return state.Stage == statusBlocked || state.Stage == statusValidationBlocked || state.Stage == statusAcceptanceContractBlocked
	}
}

// clearArchiveLock refuses to archive a run that may still be owned by a live worker.
func clearArchiveLock(repo, runID string) error {
	status, err := lockFileStatus(repo, runID, runtime.GOOS)
	if err != nil {
		return err
	}
	switch status {
	case lockStatusActive:
		return newRunLockedError(runID)
	case lockStatusUnknown:
		return fmt.Errorf("run %s 存在无法确认的 lock，不能归档", runID)
	case lockStatusStale:
		if err := os.Remove(filepath.Join(runDir(repo, runID), "lock")); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

type alreadyArchivedRunError struct {
	runID string
}

// Error returns a stable message for idempotent batch archiving.
func (err alreadyArchivedRunError) Error() string {
	return "工作流 " + err.runID + " 已归档"
}

// isAlreadyArchivedRunError lets batch archive tolerate a run archived by a previous attempt.
func isAlreadyArchivedRunError(err error) bool {
	_, ok := err.(alreadyArchivedRunError)
	return ok
}
