// Package app contains workflow engine state and execution boundaries.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"unicode/utf8"
)

var afterQualityLoopProposalRestore = func(string) error { return nil }

// Resume loads the newest unfinished run and continues from its current stage.
func (e *Engine) Resume(ctx context.Context) error {
	return e.resume(ctx, false)
}

// ResumeAfterUserChoice resumes after the interactive menu made the lock decision explicit.
func (e *Engine) ResumeAfterUserChoice(ctx context.Context) error {
	return e.resume(ctx, true)
}

// ResumeDetachedAfterUserChoice starts an unfinished run in the background after an explicit menu choice.
func (e *Engine) ResumeDetachedAfterUserChoice(ctx context.Context, runID string) error {
	_ = ctx
	state, err := loadState(e.Repo, runID)
	if err != nil {
		return err
	}
	rollback, err := captureDetachedRunRollback(e.Repo, state)
	if err != nil {
		return err
	}
	if state.Status == statusBlocked || state.Stage == statusBlocked {
		e.printProgress(state, "blocked")
		return fmt.Errorf("run %s 已到达 blocked_review_limit，无法自动继续", runID)
	}
	if state.Status == statusValidationBlocked || state.Stage == statusValidationBlocked {
		e.printProgress(state, "blocked")
		return fmt.Errorf("run %s 已到达 blocked_validation_limit，无法自动继续", runID)
	}
	status, err := clearInactiveRunLock(e.Repo, runID, runtime.GOOS, true)
	if err != nil {
		return err
	}
	if status == lockStatusActive {
		return newRunLockedError(runID)
	}
	if err := e.validateResumePrerequisites(&state); err != nil {
		return err
	}
	if err := e.prepareQualityLoopResume(&state, false); err != nil {
		e.printProgress(state, "blocked")
		return restoreStateAfterDetachedPreparationFailure(e.Repo, rollback, "resume", err)
	}
	preparedHash, err := detachedRunStateHash(e.Repo, runID)
	if err != nil {
		return restoreStateAfterDetachedPreparationFailure(e.Repo, rollback, "resume", err)
	}
	if err := startDetachedCommand(e.Repo, runID); err != nil {
		return restoreStateAfterDetachedFailure(e.Repo, rollback, preparedHash, "resume", err)
	}
	if e.stageRuntime == nil {
		e.stageRuntime = map[string]stageRuntime{}
	}
	e.stageRuntime[state.Stage] = stageRuntime{}
	e.printProgress(state, "submitted")
	return nil
}

// detachedRunRollback captures durable run state and any archived proposal location changed by preparation.
type detachedRunRollback struct {
	State                State
	ActiveProposalExists bool
	ArchivedProposalPath string
}

// captureDetachedRunRollback snapshots state and the proposal location before detached preparation.
func captureDetachedRunRollback(repo string, state State) (detachedRunRollback, error) {
	cloned, err := cloneResumeRollbackState(state)
	if err != nil {
		return detachedRunRollback{}, err
	}
	rollback := detachedRunRollback{State: cloned}
	if state.QualityLoop.BlockedFromStage != workflowStageArchive {
		return rollback, nil
	}
	active := changePath(repo, state.ChangeName)
	if info, statErr := os.Stat(active); statErr == nil {
		rollback.ActiveProposalExists = info.IsDir()
		return rollback, nil
	} else if !os.IsNotExist(statErr) {
		return detachedRunRollback{}, statErr
	}
	matches, err := filepath.Glob(filepath.Join(repo, "docs", "changes", "archive", "*-"+state.ChangeName))
	if err != nil {
		return detachedRunRollback{}, err
	}
	if len(matches) == 1 {
		rollback.ArchivedProposalPath = matches[0]
	}
	return rollback, nil
}

// restoreStateAfterDetachedPreparationFailure compensates every failure before worker launch.
func restoreStateAfterDetachedPreparationFailure(
	repo string,
	rollback detachedRunRollback,
	operation string,
	prepareErr error,
) error {
	expectedHash, err := detachedRunStateHash(repo, rollback.State.RunID)
	if err != nil {
		return errors.Join(prepareErr, fmt.Errorf("读取 %s preparation 回滚代次失败：%w", operation, err))
	}
	return restoreStateAfterDetachedFailure(repo, rollback, expectedHash, operation, prepareErr)
}

// restoreStateAfterDetachedFailure restores a snapshot only while its prepared state is still current.
func restoreStateAfterDetachedFailure(
	repo string,
	rollback detachedRunRollback,
	expectedHash string,
	operation string,
	operationErr error,
) error {
	if detachedWorkerProcessStarted(operationErr) {
		return operationErr
	}
	unlock, err := acquireLock(repo, rollback.State.RunID)
	if err != nil {
		return errors.Join(operationErr, fmt.Errorf("%s 后台启动前状态已有活动 lease，拒绝陈旧回滚：%w", operation, err))
	}
	defer unlock()
	restored, err := restoreDetachedRunIfCurrent(repo, rollback, expectedHash)
	if err != nil {
		return errors.Join(operationErr, fmt.Errorf("恢复 %s 后台启动前事务失败：%w", operation, err))
	}
	if !restored {
		return errors.Join(operationErr, fmt.Errorf("%s 后台启动前状态代次已变化，拒绝陈旧回滚", operation))
	}
	return operationErr
}

// detachedRunStateHash returns the durable generation used by detached rollback CAS.
func detachedRunStateHash(repo, runID string) (string, error) {
	stateFileMu.Lock()
	defer stateFileMu.Unlock()
	unlock, err := lockRunStateFile(repo, runID)
	if err != nil {
		return "", err
	}
	defer func() { _ = unlock() }()
	data, err := os.ReadFile(filepath.Join(runDir(repo, runID), "state.json"))
	if err != nil {
		return "", err
	}
	return qualityHashStrings(string(data)), nil
}

// restoreDetachedRunIfCurrent atomically checks the prepared generation and applies proposal/state compensation.
func restoreDetachedRunIfCurrent(repo string, rollback detachedRunRollback, expectedHash string) (bool, error) {
	stateFileMu.Lock()
	defer stateFileMu.Unlock()
	runID := rollback.State.RunID
	unlock, err := lockRunStateFile(repo, runID)
	if err != nil {
		return false, err
	}
	defer func() { _ = unlock() }()
	data, err := os.ReadFile(filepath.Join(runDir(repo, runID), "state.json"))
	if err != nil {
		return false, err
	}
	if expectedHash == "" || qualityHashStrings(string(data)) != expectedHash {
		return false, nil
	}
	if err := restoreDetachedProposalLocation(repo, rollback); err != nil {
		return false, err
	}
	normalizeStateMaps(&rollback.State)
	refreshStateProcesses(&rollback.State)
	if err := writeStateFileLocked(repo, runID, rollback.State); err != nil {
		return false, err
	}
	return true, nil
}

// restoreDetachedProposalLocation moves an internally restored archive proposal back to its original path.
func restoreDetachedProposalLocation(repo string, rollback detachedRunRollback) error {
	if rollback.ActiveProposalExists || rollback.ArchivedProposalPath == "" {
		return nil
	}
	active := changePath(repo, rollback.State.ChangeName)
	activeInfo, activeErr := os.Stat(active)
	_, archivedErr := os.Stat(rollback.ArchivedProposalPath)
	switch {
	case os.IsNotExist(activeErr) && archivedErr == nil:
		return nil
	case activeErr == nil && activeInfo.IsDir() && os.IsNotExist(archivedErr):
		return os.Rename(active, rollback.ArchivedProposalPath)
	case activeErr != nil && !os.IsNotExist(activeErr):
		return activeErr
	case archivedErr != nil && !os.IsNotExist(archivedErr):
		return archivedErr
	default:
		return fmt.Errorf("提案位置不唯一：active=%v archived=%v", activeErr, archivedErr)
	}
}

// ResumeRunJSON resumes a specific run, emits its runner DTO, then continues the workflow.
func (e *Engine) ResumeRunJSON(ctx context.Context, runID string, stdout io.Writer) error {
	return e.resumeRun(ctx, runID, false, stdout)
}

// resume loads the newest recoverable run and handles lock policy before continuing.
func (e *Engine) resume(ctx context.Context, allowUnknownLock bool) error {
	runID, err := FindUnfinishedRun(e.Repo)
	if err != nil {
		return err
	}
	if runID == "" {
		return fmt.Errorf("没有未完成 run")
	}
	return e.resumeRun(ctx, runID, allowUnknownLock, nil)
}

// resumeRun loads a specific recoverable run and handles lock policy before continuing.
func (e *Engine) resumeRun(ctx context.Context, runID string, allowUnknownLock bool, startupJSON io.Writer) error {
	return e.resumeRunWithRollback(ctx, runID, allowUnknownLock, startupJSON, nil)
}

// resumeRunWithRollback restores the selected pre-call state when startup output fails before worker ownership begins.
func (e *Engine) resumeRunWithRollback(
	ctx context.Context,
	runID string,
	allowUnknownLock bool,
	startupJSON io.Writer,
	startupRollback *State,
) error {
	state, err := loadState(e.Repo, runID)
	if err != nil {
		return err
	}
	if state.Status == statusBlocked || state.Stage == statusBlocked {
		e.printProgress(state, "blocked")
		return fmt.Errorf("run %s 已到达 blocked_review_limit，无法自动继续", runID)
	}
	if state.Status == statusValidationBlocked || state.Stage == statusValidationBlocked {
		e.printProgress(state, "blocked")
		return fmt.Errorf("run %s 已到达 blocked_validation_limit，无法自动继续", runID)
	}
	status, err := clearInactiveRunLock(e.Repo, runID, runtime.GOOS, allowUnknownLock)
	if err != nil {
		return err
	}
	if status == lockStatusActive {
		return newRunLockedError(runID)
	}
	if status == lockStatusUnknown {
		if !allowUnknownLock {
			return fmt.Errorf("run %s 存在无法确认的 lock，请通过交互菜单恢复或中止", runID)
		}
	}
	unlock, err := acquireLock(e.Repo, runID)
	if err != nil {
		return err
	}
	defer unlock()
	rollback, err := cloneResumeRollbackState(state)
	if err != nil {
		return err
	}
	if startupRollback != nil {
		rollback, err = cloneResumeRollbackState(*startupRollback)
		if err != nil {
			return err
		}
	}
	if err := e.validateResumePrerequisites(&state); err != nil {
		return err
	}
	if err := e.prepareQualityLoopResume(&state, false); err != nil {
		e.printProgress(state, "blocked")
		return err
	}
	if err := saveState(e.Repo, state); err != nil {
		return err
	}
	if startupJSON != nil {
		if err := writeRunnerState(startupJSON, state); err != nil {
			return restoreResumeStateAfterStartupWriteFailure(e.Repo, rollback, err)
		}
		flushWriter(startupJSON)
	}
	if stateUsesGoDAG(state) {
		return withRunWorkerLifecycle(e.Repo, state, func() error {
			return e.runGoDAGLocked(ctx, state)
		})
	}
	return withRunWorkerLifecycle(e.Repo, state, func() error {
		return e.runLoop(ctx, state)
	})
}

// runnerStartupWriteError distinguishes pre-worker output failures from workflow execution failures.
type runnerStartupWriteError struct {
	Cause error
}

// Error describes the startup output boundary that failed.
func (e *runnerStartupWriteError) Error() string {
	return fmt.Sprintf("写入 runner 启动状态失败：%v", e.Cause)
}

// Unwrap exposes the underlying writer error.
func (e *runnerStartupWriteError) Unwrap() error {
	return e.Cause
}

// isRunnerStartupWriteError reports whether workflow execution never acquired worker ownership.
func isRunnerStartupWriteError(err error) bool {
	var target *runnerStartupWriteError
	return errors.As(err, &target)
}

// cloneResumeRollbackState makes the rollback snapshot independent from normalized workflow maps.
func cloneResumeRollbackState(state State) (State, error) {
	data, err := json.Marshal(state)
	if err != nil {
		return State{}, fmt.Errorf("复制 resume 回滚状态失败：%w", err)
	}
	var cloned State
	if err := json.Unmarshal(data, &cloned); err != nil {
		return State{}, fmt.Errorf("复制 resume 回滚状态失败：%w", err)
	}
	return cloned, nil
}

// restoreResumeStateAfterStartupWriteFailure rolls back state before returning the writer failure.
func restoreResumeStateAfterStartupWriteFailure(repo string, rollback State, writeErr error) error {
	startupErr := &runnerStartupWriteError{Cause: writeErr}
	if err := saveState(repo, rollback); err != nil {
		return errors.Join(startupErr, fmt.Errorf("恢复 runner 启动前状态失败：%w", err))
	}
	return startupErr
}

// validateResumePrerequisites checks the sealed workflow and backend before a recoverable block is durably cleared.
func (e *Engine) validateResumePrerequisites(state *State) error {
	if state == nil {
		return fmt.Errorf("resume 缺少 run state")
	}
	if !hasWorkflowConfig(*state) {
		return fmt.Errorf("run %s 缺少 workflow_config 快照", state.RunID)
	}
	normalizeWorkflowConfig(&state.Workflow)
	state.Engine = publicEngineLabel(state.Workflow.Engine)
	return e.Registry.ResolveForWorkflow(state.Workflow)
}

// prepareQualityLoopResume restores a block after progress or an explicit restart instruction.
func (e *Engine) prepareQualityLoopResume(state *State, manualInstruction bool) error {
	if state == nil || (state.Status != statusBlockedEnvironment && state.Status != statusBlockedStalled) {
		return nil
	}
	original := state.QualityLoop.BlockedFromStage
	if original == "" {
		return fmt.Errorf("run %s 的 %s 缺少原阶段，无法恢复", state.RunID, state.Status)
	}
	blockedFrom := original
	archiveRepairGate, hasArchiveRepairGate := state.ArtifactGates[workflowStageArchive]
	hasArchiveRepairGate = hasArchiveRepairGate &&
		strings.Contains(archiveRepairGate.LastError, "archive 提升最终 acceptance evidence 失败")
	if blockedFrom == workflowStageArchive {
		resumed, err := e.resumeQualityLoopArchiveGate(state)
		if err != nil {
			return err
		}
		if resumed {
			return saveState(e.Repo, *state)
		}
	}
	switch state.Status {
	case statusBlockedEnvironment:
		names := append([]string(nil), state.QualityLoop.MissingEnvironmentNames...)
		var missing []string
		for _, name := range names {
			if !qualityEnvironmentAvailable(e.Repo, name) {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("run %s 仍缺少环境前置条件：%s", state.RunID, strings.Join(missing, ", "))
		}
		if err := e.absorbQualityEnvironmentProgress(state, names); err != nil {
			return err
		}
	case statusBlockedStalled:
		blockedDiffHash := state.QualityLoop.DiffHash
		blockedEvidenceHash := state.QualityLoop.EvidenceHash
		blockedEvidenceProgressHash := state.QualityLoop.EvidenceProgressHash
		head, diff, err := gitSnapshot(e.Repo)
		if err != nil {
			return err
		}
		probe := *state
		probe.BaselineHead = head
		probe.BaselineDiff = diff
		content, err := gitChangeContentSnapshotForChange(e.Repo, state.ChangeName)
		if err != nil {
			return err
		}
		probe.QualityLoop.DiffHash = qualityHashStrings(content)
		evidenceObservation, err := qualityCurrentEvidenceObservationForState(e.Repo, probe)
		if err != nil {
			return err
		}
		probe.QualityLoop.EvidenceHash = evidenceObservation.RawHash
		probe.QualityLoop.EvidenceProgressHash = evidenceObservation.ProgressHash
		progressProbe := probe
		progressProbe.QualityLoop.EvidenceHash = blockedEvidenceHash
		progressProbe.QualityLoop.EvidenceProgressHash = blockedEvidenceProgressHash
		progressChanged := qualityProgressHash(progressProbe) != qualityBlockedProgressHash(*state)
		evidenceChanged := evidenceObservation.ProgressEligible && blockedEvidenceProgressHash != "" &&
			probe.QualityLoop.EvidenceProgressHash != blockedEvidenceProgressHash
		if !progressChanged && !evidenceChanged && !manualInstruction {
			return fmt.Errorf("run %s 仍无新代码、证据、配置或人工指令，保持 blocked_stalled", state.RunID)
		}
		state.BaselineHead = head
		state.BaselineDiff = diff
		state.QualityLoop.DiffHash = probe.QualityLoop.DiffHash
		state.QualityLoop.EvidenceHash = probe.QualityLoop.EvidenceHash
		state.QualityLoop.EvidenceProgressHash = probe.QualityLoop.EvidenceProgressHash
		if probe.QualityLoop.DiffHash != blockedDiffHash || evidenceChanged || manualInstruction {
			original = qualityLoopResumeStageAfterQASourceProgress(e.Repo, state, original)
		}
	}
	if blockedFrom == workflowStageArchive && strings.HasPrefix(original, "audit_") {
		if err := e.restoreQualityLoopProposalForAudit(state); err != nil {
			return err
		}
		if hasArchiveRepairGate {
			archiveRepairGate.Kind = validationKindArchiveRepair
			state.ArtifactGates[original] = archiveRepairGate
			state.QualityLoop.ArchiveGateFingerprint = qualityHashStrings(
				validationKindArchiveRepair,
				strings.TrimSpace(archiveRepairGate.LastError),
			)
		}
	}
	state.Status = statusRunning
	state.Stage = original
	state.QualityLoop.ResumeRerunPending = true
	state.Error = ""
	state.QualityLoop.BlockedFromStage = ""
	state.QualityLoop.MissingEnvironmentNames = nil
	return saveState(e.Repo, *state)
}

// resumeQualityLoopArchiveGate reruns an interrupted archive without treating its move as source drift.
func (e *Engine) resumeQualityLoopArchiveGate(state *State) (bool, error) {
	if state == nil || state.Status != statusBlockedStalled {
		return false, nil
	}
	gate, ok := state.ArtifactGates[workflowStageArchive]
	if !ok || gate.Kind != validationKindArchiveReadOnly ||
		strings.Contains(gate.LastError, "archive 提升最终 acceptance evidence 失败") {
		return false, nil
	}
	state.Stages[workflowStageArchive] = statusRunning
	state.Status = statusRunning
	state.Stage = workflowStageArchive
	state.Error = ""
	state.QualityLoop.BlockedFromStage = ""
	state.QualityLoop.MissingEnvironmentNames = nil
	state.QualityLoop.ResumeRerunPending = false
	return true, nil
}

// restoreQualityLoopProposalForAudit moves one verified archived proposal back to its active path.
func (e *Engine) restoreQualityLoopProposalForAudit(state *State) error {
	if state == nil {
		return fmt.Errorf("archive 恢复缺少 run state")
	}
	active := filepath.Join(e.Repo, "docs", "changes", state.ChangeName)
	info, err := os.Stat(active)
	restored := false
	archivedDir := ""
	switch {
	case err == nil && !info.IsDir():
		return fmt.Errorf("archive 恢复目标不是目录：%s", active)
	case err == nil:
	case !os.IsNotExist(err):
		return err
	default:
		matches, globErr := filepath.Glob(filepath.Join(e.Repo, "docs", "changes", "archive", "*-"+state.ChangeName))
		if globErr != nil {
			return globErr
		}
		if len(matches) != 1 {
			return fmt.Errorf("archive 恢复要求唯一归档提案，找到 %d 个：%s", len(matches), state.ChangeName)
		}
		archivedDir = matches[0]
		if err := verifyArchivedAcceptanceMatchesSealed(e.Repo, archivedDir, state.ChangeName, state.AcceptanceHash); err != nil {
			return fmt.Errorf("archive 恢复拒绝非封存提案：%w", err)
		}
		if err := os.Rename(archivedDir, active); err != nil {
			return fmt.Errorf("archive 恢复提案失败：%w", err)
		}
		restored = true
	}
	if restored {
		if err := restoreQualityLoopArchivedReferences(e.Repo, active, archivedDir, state.ChangeName); err != nil {
			return err
		}
		if err := afterQualityLoopProposalRestore(active); err != nil {
			return err
		}
	}
	if err := verifyAcceptanceMatchesSealed(filepath.Join(active, "acceptance.json"), state.AcceptanceHash); err != nil {
		return err
	}
	head, diff, err := gitSnapshot(e.Repo)
	if err != nil {
		return err
	}
	content, err := gitChangeContentSnapshotForChange(e.Repo, state.ChangeName)
	if err != nil {
		return err
	}
	state.BaselineHead = head
	state.BaselineDiff = diff
	state.QualityLoop.DiffHash = qualityHashStrings(content)
	return nil
}

// restoreQualityLoopArchivedReferences converts deterministic archive paths back to active proposal paths.
func restoreQualityLoopArchivedReferences(repo, activeDir, archivedDir, changeName string) error {
	return filepath.WalkDir(activeDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !utf8.Valid(data) {
			return nil
		}
		normalized, err := normalizeQualityLoopArchivedReferences(repo, activeDir, archivedDir, changeName, data)
		if err != nil {
			return err
		}
		if string(normalized) == string(data) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(path, normalized, info.Mode().Perm())
	})
}

// normalizeQualityLoopArchivedReferences reverses archive paths and repairs legacy root-test rewrites.
func normalizeQualityLoopArchivedReferences(repo, proposalDir, archivedDir, changeName string, data []byte) ([]byte, error) {
	relativeArchive, err := filepath.Rel(repo, archivedDir)
	if err != nil {
		return nil, err
	}
	archivePrefix := strings.TrimPrefix(filepath.ToSlash(relativeArchive), "./") + "/"
	activePrefix := "docs/changes/" + changeName + "/"
	testReference := regexp.MustCompile(regexp.QuoteMeta(archivePrefix+"tests/") + `[^\s\x60"'()\[\]{}<>，。；：,]+`)
	text := testReference.ReplaceAllStringFunc(string(data), func(reference string) string {
		relative := strings.TrimPrefix(reference, archivePrefix)
		if _, statErr := os.Stat(filepath.Join(proposalDir, filepath.FromSlash(relative))); os.IsNotExist(statErr) {
			return relative
		}
		return reference
	})
	return []byte(strings.ReplaceAll(text, archivePrefix, activePrefix)), nil
}

// qualityLoopResumeStageAfterQASourceProgress routes post-checkpoint progress through fresh gates.
func qualityLoopResumeStageAfterQASourceProgress(repo string, state *State, original string) string {
	if state == nil {
		return original
	}
	stage, err := parseWorkflowStage(original)
	if err != nil {
		return original
	}
	if stage.isKind(workflowStageQA) {
		qaPath := filepath.Join(runDir(repo, state.RunID), fmt.Sprintf("qa-%d.json", stage.Iteration))
		qa, qaErr := ReadQA(qaPath)
		contract, contractErr := readAcceptanceForState(repo, *state)
		if qaErr == nil && contractErr == nil && QANeedsFix(qa) &&
			ValidateQAAgainstAcceptance(qa, contract) == nil &&
			qualityLoopTrustedSourceQA(repo, *state, original, qa) {
			target := fmt.Sprintf("targeted_repair_%d", stage.Iteration)
			state.QualityLoop.Mode = "qa_targeted_repair"
			state.QualityLoop.SourceQAArtifact = fmt.Sprintf("qa-%d.json", stage.Iteration)
			state.QualityLoop.SourceQAHash = qaArtifactContentHash(qa)
			state.QualityLoop.FindingFingerprint = qaFindingFingerprint(qa)
			state.QualityLoop.ProgressHash = qualityProgressHash(*state)
			state.QualityLoop.RerunFindingFingerprint = ""
			state.QualityLoop.RerunProgressHash = ""
			state.QualityLoop.GateFailureFingerprint = ""
			state.QualityLoop.GateProgressHash = ""
			state.QualityLoop.RequiredTestsPassed = false
			state.QualityLoop.FailedTestsReplayed = false
			return target
		}
		return qualityLoopResumeAuditStage(state)
	}
	if !stage.isKind(workflowStageArchive) {
		return original
	}
	return qualityLoopResumeAuditStage(state)
}

// qualityLoopResumeAuditStage starts a fresh audit after unreviewed source progress.
func qualityLoopResumeAuditStage(state *State) string {
	latest := 0
	for name := range state.Stages {
		stage, err := parseWorkflowStage(name)
		if err == nil && stage.isKind(workflowStageAudit) && stage.Iteration > latest {
			latest = stage.Iteration
		}
	}
	state.QualityLoop.Mode = "pre_qa_audit"
	state.QualityLoop.SourceQAArtifact = ""
	state.QualityLoop.SourceQAHash = ""
	state.QualityLoop.FindingFingerprint = ""
	state.QualityLoop.ProgressHash = ""
	state.QualityLoop.RerunFindingFingerprint = ""
	state.QualityLoop.RerunProgressHash = ""
	state.QualityLoop.GateFailureFingerprint = ""
	state.QualityLoop.GateProgressHash = ""
	state.QualityLoop.RequiredTestsPassed = false
	state.QualityLoop.FailedTestsReplayed = false
	if state.ArtifactGates != nil {
		delete(state.ArtifactGates, workflowStageArchive)
	}
	return fmt.Sprintf("audit_%d", latest+1)
}

// qualityLoopTrustedSourceQA requires a completed read-only gate bound to durable tested input.
func qualityLoopTrustedSourceQA(repo string, state State, stageName string, qa QA) bool {
	stage, err := parseWorkflowStage(stageName)
	if err != nil || !stage.isKind(workflowStageQA) ||
		state.QualityLoop.SourceQAArtifact != fmt.Sprintf("qa-%d.json", stage.Iteration) ||
		state.QualityLoop.SourceQAHash == "" ||
		state.QualityLoop.SourceQAHash != qaArtifactContentHash(qa) {
		return false
	}
	gate := state.ArtifactGates[stageName]
	if gate.Kind != validationKindQAReadOnly || gate.Status != validationStatusPassed ||
		gate.DiffHash == "" || gate.CheckpointHash == "" {
		return false
	}
	probe := state
	probe.Stage = stageName
	checkpoint, expectedHash, err := qualityLoopQACheckpointDiffHash(probe)
	if err != nil || gate.DiffHash != expectedHash {
		return false
	}
	checkpointHash, err := qualityLoopCheckpointTrustHash(repo, probe, checkpoint)
	if err != nil || checkpointHash != gate.CheckpointHash {
		return false
	}
	probe, err = qualityLoopDurableCheckpointProbe(repo, probe, checkpoint)
	if err != nil || probe.QualityLoop.DiffHash != expectedHash {
		return false
	}
	if verifyQualityLoopDurableCheckpoint(repo, probe, checkpoint) != nil {
		return false
	}
	verifiedHash, err := qualityLoopCheckpointTrustHash(repo, probe, checkpoint)
	return err == nil && verifiedHash == checkpointHash
}

// absorbQualityEnvironmentProgress accepts only declared repository prerequisites into the run baseline.
func (e *Engine) absorbQualityEnvironmentProgress(state *State, names []string) error {
	head, diff, err := gitSnapshot(e.Repo)
	if err != nil {
		return err
	}
	paths, err := qualityExpandResumePaths(e.Repo, changedStatusPaths(state.BaselineDiff, diff))
	if err != nil {
		return err
	}
	currentContent, err := gitStatusContentSnapshot(e.Repo, diff)
	if err != nil {
		return err
	}
	if state.QualityLoop.EnvironmentContent == "" && len(statusLineByPath(state.BaselineDiff)) > 0 {
		return fmt.Errorf("run %s 的环境阻塞缺少 dirty 内容检查点，拒绝吸收恢复变化", state.RunID)
	}
	paths = append(paths, gitContentSnapshotChangedPaths(state.QualityLoop.EnvironmentContent, currentContent)...)
	if state.BaselineHead != "" && state.BaselineHead != head {
		before, err := qualityCanonicalCommit(e.Repo, state.BaselineHead)
		if err != nil {
			return err
		}
		if before != head {
			committed, err := committedPaths(e.Repo, before, head)
			if err != nil {
				return err
			}
			if len(committed) == 0 {
				return fmt.Errorf("run %s 恢复环境时检测到未绑定路径的 HEAD 变化", state.RunID)
			}
			paths = append(paths, committed...)
		}
	}
	allowed := qualityEnvironmentRepositoryPaths(e.Repo, names)
	var blocked []string
	for _, path := range uniqueSortedPaths(paths) {
		if !qualityEnvironmentPathAllowed(path, allowed) {
			blocked = append(blocked, path)
		}
	}
	if len(blocked) > 0 {
		return fmt.Errorf("run %s 恢复环境时检测到未声明路径变化：%s", state.RunID, strings.Join(blocked, ", "))
	}
	content, err := gitChangeContentSnapshotForChange(e.Repo, state.ChangeName)
	if err != nil {
		return err
	}
	state.BaselineHead = head
	state.BaselineDiff = diff
	state.QualityLoop.DiffHash = qualityHashStrings(content)
	state.QualityLoop.EnvironmentContent = ""
	return nil
}

// qualityExpandResumePaths expands newly untracked directory summaries so every changed file is checked.
func qualityExpandResumePaths(repo string, paths []string) ([]string, error) {
	gitPath, err := resolveCommand("git")
	if err != nil {
		return nil, err
	}
	var expanded []string
	for _, path := range paths {
		if !strings.HasSuffix(filepath.ToSlash(path), "/") {
			expanded = append(expanded, path)
			continue
		}
		cmd := commandContext(context.Background(), gitPath, "-c", "core.quotePath=false", "ls-files", "--others", "--exclude-standard", "--", path)
		cmd.Dir = repo
		output, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("展开恢复路径 %q 失败：%s", path, strings.TrimSpace(string(output)))
		}
		items := statusNamePaths(string(output))
		if len(items) == 0 {
			expanded = append(expanded, path)
			continue
		}
		expanded = append(expanded, items...)
	}
	return expanded, nil
}

// qualityCanonicalCommit resolves saved symbolic test heads and durable commit hashes uniformly.
func qualityCanonicalCommit(repo, revision string) (string, error) {
	gitPath, err := resolveCommand("git")
	if err != nil {
		return "", err
	}
	cmd := commandContext(context.Background(), gitPath, "rev-parse", "--verify", revision+"^{commit}")
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("解析恢复 baseline HEAD %q 失败：%s", revision, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

// qualityEnvironmentRepositoryPath identifies one declared path and whether descendants are allowed.
type qualityEnvironmentRepositoryPath struct {
	path      string
	directory bool
}

// qualityEnvironmentRepositoryPaths resolves declared non-ENV prerequisites inside the repository.
func qualityEnvironmentRepositoryPaths(repo string, names []string) []qualityEnvironmentRepositoryPath {
	var paths []qualityEnvironmentRepositoryPath
	for _, name := range names {
		if isConventionalEnvironmentName(name) {
			continue
		}
		full := strings.TrimSpace(name)
		if !filepath.IsAbs(full) {
			full = filepath.Join(repo, full)
		}
		relative, err := filepath.Rel(repo, full)
		if err != nil || relative == "." || filepath.IsAbs(relative) ||
			relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		info, err := os.Stat(full)
		if err != nil {
			continue
		}
		paths = append(paths, qualityEnvironmentRepositoryPath{
			path:      filepath.ToSlash(relative),
			directory: info.IsDir(),
		})
	}
	return paths
}

// qualityEnvironmentPathAllowed checks exact files and descendants of declared prerequisite directories.
func qualityEnvironmentPathAllowed(path string, allowed []qualityEnvironmentRepositoryPath) bool {
	path = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(path)), "./")
	for _, item := range allowed {
		if path == item.path || (item.directory && strings.HasPrefix(path, strings.TrimSuffix(item.path, "/")+"/")) {
			return true
		}
	}
	return false
}

// qualityEnvironmentAvailable checks a variable or repository-relative path without persisting its value.
func qualityEnvironmentAvailable(repo, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	if isConventionalEnvironmentName(name) {
		return nonEmptyEnvironmentValue(name)
	}
	path := name
	if !filepath.IsAbs(path) {
		path = filepath.Join(repo, path)
	}
	if _, err := os.Stat(path); err == nil {
		return true
	}
	return nonEmptyEnvironmentValue(name)
}

// isConventionalEnvironmentName identifies standard upper-case variable names without treating them as files.
func isConventionalEnvironmentName(name string) bool {
	for i, r := range name {
		if (r >= 'A' && r <= 'Z') || r == '_' || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return name != ""
}

// nonEmptyEnvironmentValue checks only presence and never persists the environment value.
func nonEmptyEnvironmentValue(name string) bool {
	value, ok := os.LookupEnv(name)
	return ok && strings.TrimSpace(value) != ""
}
