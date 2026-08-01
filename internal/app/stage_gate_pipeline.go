// Package app centralizes deterministic gates that complete one main workflow stage.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type stageGatePipelineMode string

const (
	stageGatePipelineLoop         stageGatePipelineMode = "loop"
	stageGatePipelineNode         stageGatePipelineMode = "node"
	validationKindArchiveReadOnly                       = "archive_read_only"
	validationKindArchiveRepair                         = "archive_repair"
)

// stageGatePipelineResult describes the caller-visible outcome of completing a stage.
type stageGatePipelineResult struct {
	Done          bool
	Blocked       bool
	ProgressLabel string
}

// qualityLoopQAGateInput freezes every source boundary before an independent QA turn starts.
type qualityLoopQAGateInput struct {
	DiffHash       string
	CheckpointHash string
}

// completeMainStage runs artifact, acceptance, validation, completion, and advance gates once.
func (e *Engine) completeMainStage(ctx context.Context, state *State, mode stageGatePipelineMode) (stageGatePipelineResult, error) {
	if e.routeUntrustedQualityLoopTargetedRepair(state) {
		return stageGatePipelineResult{Done: true, ProgressLabel: "rerouted"}, nil
	}
	done, err := e.checkStageArtifactGate(*state)
	if err != nil {
		return stageGatePipelineResult{}, err
	}
	if !done {
		return stageGatePipelineResult{}, nil
	}
	if isQualityLoopRepairStage(*state) {
		if contractErr := e.verifyQualityLoopActiveAcceptance(*state); contractErr != nil {
			return stageGatePipelineResult{}, e.stageArtifactGateError(*state, contractErr)
		}
		artifact, _, artifactErr := e.validateStageArtifact(*state)
		if artifactErr != nil {
			return stageGatePipelineResult{}, artifactErr
		}
		if names := qualityEnvironmentNamesFromRepair(artifact.Repair); len(names) > 0 {
			if err := blockQualityEnvironment(e.Repo, state, names); err != nil {
				return stageGatePipelineResult{}, err
			}
			return stageGatePipelineResult{Done: true, Blocked: true, ProgressLabel: "blocked"}, nil
		}
	}
	if isQualityLoopQAStage(*state) {
		artifact, _, artifactErr := e.validateStageArtifact(*state)
		if artifactErr != nil {
			return stageGatePipelineResult{}, artifactErr
		}
		if names := qualityEnvironmentNamesFromQA(artifact.QA); len(names) > 0 {
			if err := blockQualityEnvironment(e.Repo, state, names); err != nil {
				return stageGatePipelineResult{}, err
			}
			return stageGatePipelineResult{Done: true, Blocked: true, ProgressLabel: "blocked"}, nil
		}
	}
	qaReadOnlyPassed, err := e.verifyQualityLoopQAReadOnlyGate(state)
	if err != nil {
		return stageGatePipelineResult{}, err
	}
	if !qaReadOnlyPassed {
		return stageGatePipelineResult{Done: true, Blocked: true, ProgressLabel: "blocked"}, nil
	}
	archiveReadOnlyPassed, err := e.verifyQualityLoopArchiveReadOnlyGate(state)
	if err != nil {
		return stageGatePipelineResult{}, err
	}
	if !archiveReadOnlyPassed {
		return stageGatePipelineResult{Done: true, Blocked: true, ProgressLabel: "blocked"}, nil
	}
	clearStageArtifactGateFailure(state)
	validationPassed, err := e.validateStage(ctx, state)
	if err != nil {
		return stageGatePipelineResult{}, err
	}
	if !validationPassed {
		return stageGatePipelineResult{Done: true, Blocked: true, ProgressLabel: failedGateProgressLabel(*state)}, nil
	}
	diffUnchanged, err := e.verifyQualityAcceptanceInputDiff(state)
	if err != nil {
		return stageGatePipelineResult{}, err
	}
	if !diffUnchanged {
		return stageGatePipelineResult{Done: true, Blocked: true, ProgressLabel: failedGateProgressLabel(*state)}, nil
	}
	if state.Stage == workflowStageExecution {
		preflightPassed, err := e.runAcceptancePreflight(state)
		if err != nil {
			return stageGatePipelineResult{}, err
		}
		if !preflightPassed {
			return stageGatePipelineResult{Done: true, Blocked: true, ProgressLabel: failedGateProgressLabel(*state)}, nil
		}
	}
	acceptancePassed, err := e.runAcceptanceGate(ctx, state)
	if err != nil {
		return stageGatePipelineResult{}, err
	}
	if !acceptancePassed {
		return stageGatePipelineResult{Done: true, Blocked: true, ProgressLabel: failedGateProgressLabel(*state)}, nil
	}
	bound, err := e.bindQualityStageGateSnapshot(state)
	if err != nil {
		return stageGatePipelineResult{}, err
	}
	if !bound {
		return stageGatePipelineResult{Done: true, Blocked: true, ProgressLabel: failedGateProgressLabel(*state)}, nil
	}
	if !normalizeRunStatus(state.Status).isRunning() {
		return stageGatePipelineResult{Done: true, Blocked: true, ProgressLabel: "blocked"}, nil
	}
	clearQualityGateFailure(state)
	markStageCompleted(state)
	if shouldAdvanceAfterMainStage(*state, mode) {
		if err := e.advance(state); err != nil {
			return stageGatePipelineResult{}, err
		}
	}
	return stageGatePipelineResult{Done: true, ProgressLabel: "next"}, nil
}

// verifyQualityAcceptanceInputDiff rejects validation commands that changed the source to be tested.
func (e *Engine) verifyQualityAcceptanceInputDiff(state *State) (bool, error) {
	if state == nil || !isQualityLoopRepairStage(*state) {
		return true, nil
	}
	currentHash, err := e.currentQualityLoopDiffHash(*state)
	if err != nil {
		return false, err
	}
	if currentHash == state.QualityLoop.DiffHash {
		return true, nil
	}
	if err := e.absorbQualityGateSourceSnapshot(state); err != nil {
		return false, err
	}
	reason := "validation changed the tracked source snapshot; rerun the stage before acceptance"
	current := state.Validation[state.Stage]
	current.Status = validationStatusFailed
	current.LastError = reason
	current.DiffHash = currentHash
	state.Validation[state.Stage] = current
	recordQualityGateFailure(state, "diff_binding", reason)
	if normalizeRunStatus(state.Status).isRunning() {
		state.Stages[state.Stage] = "validation_failed"
	}
	return false, nil
}

// prepareQualityLoopQAReadOnlyGate binds a QA turn to the latest tested repair diff.
func (e *Engine) prepareQualityLoopQAReadOnlyGate(state *State) (qualityLoopQAGateInput, bool, error) {
	if state == nil || !isQualityLoopQAStage(*state) {
		return qualityLoopQAGateInput{}, false, nil
	}
	currentHash, err := e.currentQualityLoopDiffHash(*state)
	if err != nil {
		return qualityLoopQAGateInput{}, false, err
	}
	checkpoint, expectedHash, err := qualityLoopQACheckpointDiffHash(*state)
	if err != nil {
		if blockErr := e.blockQualityLoopQAReadOnly(state, currentHash, err.Error()); blockErr != nil {
			return qualityLoopQAGateInput{}, false, blockErr
		}
		return qualityLoopQAGateInput{}, true, nil
	}
	checkpointHash, err := qualityLoopCheckpointTrustHash(e.Repo, *state, checkpoint)
	if err != nil {
		if blockErr := e.blockQualityLoopQAReadOnly(state, currentHash, err.Error()); blockErr != nil {
			return qualityLoopQAGateInput{}, false, blockErr
		}
		return qualityLoopQAGateInput{}, true, nil
	}
	if err := verifyQualityLoopDurableCheckpoint(e.Repo, *state, checkpoint); err != nil {
		if rerouteQualityLoopQAOnValidationDiffDrift(state, err) {
			return qualityLoopQAGateInput{}, true, nil
		}
		if e.rerouteQualityLoopQAOnAcceptanceDrift(state, err) {
			return qualityLoopQAGateInput{}, true, nil
		}
		if blockErr := e.blockQualityLoopQAReadOnly(state, currentHash, err.Error()); blockErr != nil {
			return qualityLoopQAGateInput{}, false, blockErr
		}
		return qualityLoopQAGateInput{}, true, nil
	}
	verifiedHash, err := qualityLoopCheckpointTrustHash(e.Repo, *state, checkpoint)
	if err != nil || verifiedHash != checkpointHash {
		reason := checkpoint + " 检查点在 QA 准备期间变化"
		if blockErr := e.blockQualityLoopQAReadOnly(state, currentHash, reason); blockErr != nil {
			return qualityLoopQAGateInput{}, false, blockErr
		}
		return qualityLoopQAGateInput{}, true, nil
	}
	if state.QualityLoop.DiffHash == "" || state.QualityLoop.DiffHash != expectedHash {
		reason := fmt.Sprintf("%s 的可信 diff 未绑定最近通过门禁的 %s", state.Stage, checkpoint)
		if blockErr := e.blockQualityLoopQAReadOnly(state, currentHash, reason); blockErr != nil {
			return qualityLoopQAGateInput{}, false, blockErr
		}
		return qualityLoopQAGateInput{}, true, nil
	}
	if currentHash != expectedHash {
		reason := fmt.Sprintf("%s 开始前 tracked source 已偏离最近通过门禁的 %s", state.Stage, checkpoint)
		if blockErr := e.blockQualityLoopQAReadOnly(state, currentHash, reason); blockErr != nil {
			return qualityLoopQAGateInput{}, false, blockErr
		}
		return qualityLoopQAGateInput{}, true, nil
	}
	return qualityLoopQAGateInput{DiffHash: currentHash, CheckpointHash: checkpointHash}, false, nil
}

// rerouteQualityLoopQAOnValidationDiffDrift turns a stale audit checkpoint into a fresh audit.
func rerouteQualityLoopQAOnValidationDiffDrift(state *State, cause error) bool {
	if state == nil || !isQualityLoopQAStage(*state) ||
		!strings.Contains(cause.Error(), "validation diff 绑定不一致") {
		return false
	}
	previousStage := state.Stage
	state.Stage = qualityLoopResumeAuditStage(state)
	state.Status = statusRunning
	state.Error = ""
	state.QualityLoop.ResumeRerunPending = true
	state.Stages[previousStage] = "rerouted"
	delete(state.StageTimings, previousStage)
	delete(state.DAGNodes, previousStage)
	delete(state.Validation, previousStage)
	delete(state.AcceptanceRun, previousStage)
	delete(state.ArtifactGates, previousStage)
	return true
}

// armQualityLoopQAReadOnlyGate records the input snapshot after retry prompts consume prior gate failures.
func armQualityLoopQAReadOnlyGate(state *State, input qualityLoopQAGateInput) {
	if state == nil || input.DiffHash == "" || input.CheckpointHash == "" {
		return
	}
	if state.ArtifactGates == nil {
		state.ArtifactGates = map[string]StageValidationState{}
	}
	gate := state.ArtifactGates[state.Stage]
	gate.Kind = validationKindQAReadOnly
	gate.DiffHash = input.DiffHash
	gate.CheckpointHash = input.CheckpointHash
	state.ArtifactGates[state.Stage] = gate
}

// verifyQualityLoopQAReadOnlyGate rejects QA output when its tracked source snapshot changed.
func (e *Engine) verifyQualityLoopQAReadOnlyGate(state *State) (bool, error) {
	if state == nil || !isQualityLoopQAStage(*state) {
		return true, nil
	}
	currentHash, err := e.currentQualityLoopDiffHash(*state)
	if err != nil {
		return false, err
	}
	checkpoint, expectedHash, err := qualityLoopQACheckpointDiffHash(*state)
	if err != nil {
		return false, e.blockQualityLoopQAReadOnly(state, currentHash, err.Error())
	}
	checkpointHash, err := qualityLoopCheckpointTrustHash(e.Repo, *state, checkpoint)
	if err != nil {
		return false, e.blockQualityLoopQAReadOnly(state, currentHash, err.Error())
	}
	if err := verifyQualityLoopDurableCheckpoint(e.Repo, *state, checkpoint); err != nil {
		if e.rerouteQualityLoopQAOnAcceptanceDrift(state, err) {
			return false, nil
		}
		return false, e.blockQualityLoopQAReadOnly(state, currentHash, err.Error())
	}
	input := state.ArtifactGates[state.Stage]
	switch {
	case input.Kind != validationKindQAReadOnly || input.DiffHash == "" || input.CheckpointHash == "":
		return false, e.blockQualityLoopQAReadOnly(state, currentHash, state.Stage+" 缺少 QA 只读输入快照")
	case input.CheckpointHash != checkpointHash:
		return false, e.blockQualityLoopQAReadOnly(state, currentHash, checkpoint+" 检查点在 QA 执行期间变化")
	case input.DiffHash != currentHash:
		return false, e.blockQualityLoopQAReadOnly(
			state,
			currentHash,
			fmt.Sprintf("%s 修改了 tracked source；QA 必须保持只读", state.Stage),
		)
	case expectedHash != currentHash || state.QualityLoop.DiffHash != expectedHash:
		return false, e.blockQualityLoopQAReadOnly(
			state,
			currentHash,
			fmt.Sprintf("%s 未绑定最近通过门禁的 %s diff", state.Stage, checkpoint),
		)
	default:
		verifiedHash, err := qualityLoopCheckpointTrustHash(e.Repo, *state, checkpoint)
		if err != nil || verifiedHash != checkpointHash {
			return false, e.blockQualityLoopQAReadOnly(state, currentHash, checkpoint+" 检查点在 QA 门禁期间变化")
		}
		input.Status = validationStatusPassed
		input.LastError = ""
		input.CheckpointHash = checkpointHash
		state.ArtifactGates[state.Stage] = input
		return true, nil
	}
}

// rerouteQualityLoopQAOnAcceptanceDrift sends changed valid acceptance input through a fresh audit.
func (e *Engine) rerouteQualityLoopQAOnAcceptanceDrift(state *State, cause error) bool {
	if state == nil || !isQualityLoopQAStage(*state) {
		return false
	}
	var drift *qualityAcceptanceCheckpointDriftError
	if !errors.As(cause, &drift) {
		return false
	}
	previousStage := state.Stage
	state.Stage = qualityLoopResumeAuditStage(state)
	state.Status = statusRunning
	state.Error = ""
	state.QualityLoop.ResumeRerunPending = true
	// Preserve the consumed QA iteration so the fresh audit cannot allocate its
	// still-present artifact path again.
	state.Stages[previousStage] = "rerouted"
	delete(state.StageTimings, previousStage)
	delete(state.DAGNodes, previousStage)
	delete(state.Validation, previousStage)
	delete(state.AcceptanceRun, previousStage)
	delete(state.ArtifactGates, previousStage)
	return true
}

// qualityLoopQACheckpointDiffHash resolves the tested audit or targeted-repair input for one QA.
func qualityLoopQACheckpointDiffHash(state State) (string, string, error) {
	stage, err := parseWorkflowStage(state.Stage)
	if err != nil || !stage.isKind(workflowStageQA) {
		return "", "", fmt.Errorf("阶段 %q 不是 quality-loop QA", state.Stage)
	}
	checkpoint := ""
	if stage.Iteration > 1 {
		targeted := fmt.Sprintf("targeted_repair_%d", stage.Iteration-1)
		if state.Stages[targeted] == "completed" {
			checkpoint = targeted
		}
	}
	if checkpoint == "" {
		latestAudit := 0
		for name, status := range state.Stages {
			parsed, parseErr := parseWorkflowStage(name)
			if status == "completed" && parseErr == nil && parsed.isKind(workflowStageAudit) && parsed.Iteration > latestAudit {
				latestAudit = parsed.Iteration
			}
		}
		if latestAudit > 0 {
			checkpoint = fmt.Sprintf("audit_%d", latestAudit)
		}
	}
	if checkpoint == "" || state.Stages[checkpoint] != "completed" {
		return "", "", fmt.Errorf("%s 缺少最近完成的 repair/audit 门禁", state.Stage)
	}
	validation := state.Validation[checkpoint]
	acceptance := state.AcceptanceRun[checkpoint]
	if validation.Status != validationStatusPassed || acceptance.Status != validationStatusPassed {
		return "", "", fmt.Errorf("%s 的 %s 尚未通过 validation 与 acceptance 门禁", state.Stage, checkpoint)
	}
	if validation.DiffHash == "" || acceptance.DiffHash == "" || validation.DiffHash != acceptance.DiffHash {
		return "", "", fmt.Errorf("%s 的 %s 缺少一致的已测试 diff 绑定", state.Stage, checkpoint)
	}
	return checkpoint, validation.DiffHash, nil
}

// verifyQualityLoopDurableCheckpoint replays validation, test-log, result, and evidence trust checks.
func verifyQualityLoopDurableCheckpoint(repo string, state State, checkpoint string) error {
	if _, err := verifyQualityValidationCheckpoint(repo, state, checkpoint); err != nil {
		return err
	}
	if _, err := verifyQualityAcceptanceCheckpoint(repo, state, checkpoint); err != nil {
		return err
	}
	return nil
}

// qualityLoopCheckpointTrustHash binds a QA gate to the exact persisted checkpoint artifacts it reviewed.
func qualityLoopCheckpointTrustHash(repo string, state State, checkpoint string) (string, error) {
	validation := state.Validation[checkpoint]
	validationData, err := readQualityValidationArtifact(validation.LastArtifact)
	if err != nil {
		return "", fmt.Errorf("%s validation 检查点不可读: %w", checkpoint, err)
	}
	acceptance := state.AcceptanceRun[checkpoint]
	acceptanceData, err := readSealedAcceptanceArtifact(acceptance.LastArtifact)
	if err != nil {
		return "", fmt.Errorf("%s acceptance 检查点不可读: %w", checkpoint, err)
	}
	return qualityHashStrings(string(validationData), string(acceptanceData)), nil
}

// qualityLoopDurableCheckpointProbe restores checkpoint-local hashes before replaying durable trust checks.
func qualityLoopDurableCheckpointProbe(repo string, state State, checkpoint string) (State, error) {
	validation := state.Validation[checkpoint]
	data, err := readQualityValidationArtifact(validation.LastArtifact)
	if err != nil {
		return State{}, err
	}
	var attempt ValidationAttempt
	if err := decodeStrictArtifactJSON(data, &attempt); err != nil {
		return State{}, err
	}
	acceptance := state.AcceptanceRun[checkpoint]
	data, err = readSealedAcceptanceArtifact(acceptance.LastArtifact)
	if err != nil {
		return State{}, err
	}
	var result AcceptanceRunResult
	if err := decodeStrictArtifactJSON(data, &result); err != nil {
		return State{}, err
	}
	probe := state
	probe.QualityLoop.DiffHash = validation.DiffHash
	probe.QualityLoop.ValidationHash = qualityValidationProgressHash(attempt)
	probe.QualityLoop.TestsHash = result.TestsHash
	probe.QualityLoop.EvidenceHash = result.EvidenceHash
	return probe, nil
}

// currentQualityLoopDiffHash hashes current-change source while excluding runtime and sibling proposals.
func (e *Engine) currentQualityLoopDiffHash(state State) (string, error) {
	content, err := gitChangeContentSnapshotForChange(e.Repo, state.ChangeName)
	if err != nil {
		return "", err
	}
	return qualityHashStrings(content), nil
}

// prepareQualityLoopArchiveReadOnlyGate prepares evidence promotion after the final trusted QA.
// Archive itself only moves the sealed proposal and rewrites deterministic paths, so it does not
// take a second source snapshot that could reject those expected filesystem changes.
func (e *Engine) prepareQualityLoopArchiveReadOnlyGate(state *State) (bool, error) {
	if state == nil || !isQualityLoopArchiveStage(*state) {
		return false, nil
	}
	if err := verifyQualityLoopTrustedFinalQA(e.Repo, *state); err != nil {
		return true, e.blockQualityLoopArchiveReadOnly(state, err.Error())
	}
	checkpoint, err := qualityLoopArchiveCheckpointStage(*state)
	if err != nil {
		return true, e.blockQualityLoopArchiveReadOnly(state, err.Error())
	}
	if err := promoteQualityAcceptanceEvidence(e.Repo, *state, checkpoint); err != nil {
		return true, e.rerouteQualityLoopArchiveRepair(
			state,
			fmt.Sprintf("archive 提升最终 acceptance evidence 失败: %v", err),
		)
	}
	state.QualityLoop.ArchiveGateFingerprint = ""
	return false, nil
}

// verifyQualityLoopArchiveReadOnlyGate verifies final QA and sealed evidence after archive moves files.
func (e *Engine) verifyQualityLoopArchiveReadOnlyGate(state *State) (bool, error) {
	if state == nil || !isQualityLoopArchiveStage(*state) {
		return true, nil
	}
	if err := verifyQualityLoopTrustedFinalQA(e.Repo, *state); err != nil {
		return false, e.blockQualityLoopArchiveReadOnly(state, err.Error())
	}
	if err := verifyQualityLoopArchivedEvidenceCommit(e.Repo, *state); err != nil {
		return false, e.blockQualityLoopArchiveReadOnly(state, err.Error())
	}
	return true, nil
}

// verifyQualityLoopTrustedFinalQA binds archive to the exact clean QA that passed read-only review.
func verifyQualityLoopTrustedFinalQA(repo string, state State) error {
	iteration := latestCompletedQAIteration(state)
	if iteration < 1 {
		return fmt.Errorf("archive 门禁缺少已完成的独立 QA")
	}
	stage := fmt.Sprintf("qa_%d", iteration)
	path := filepath.Join(runDir(repo, state.RunID), fmt.Sprintf("qa-%d.json", iteration))
	qa, err := ReadQA(path)
	if err != nil {
		return fmt.Errorf("archive 门禁读取最终 QA 失败: %w", err)
	}
	contract, err := readAcceptanceForState(repo, state)
	if err != nil {
		return fmt.Errorf("archive 门禁读取封存 acceptance 失败: %w", err)
	}
	if err := ValidateQAAgainstAcceptance(qa, contract); err != nil {
		return fmt.Errorf("archive 门禁最终 QA 未覆盖封存 acceptance: %w", err)
	}
	if QANeedsFix(qa) {
		return fmt.Errorf("archive 门禁最终 QA 仍需修复")
	}
	if !qualityLoopTrustedSourceQA(repo, state, stage, qa) {
		return fmt.Errorf("archive 门禁最终 QA 未通过已测试输入信任校验")
	}
	return nil
}

// qualityLoopArchiveCheckpointStage resolves the tested repair/audit immediately preceding final QA.
func qualityLoopArchiveCheckpointStage(state State) (string, error) {
	iteration := latestCompletedQAIteration(state)
	if iteration < 1 {
		return "", fmt.Errorf("archive 门禁缺少已完成的独立 QA")
	}
	probe := state
	probe.Stage = fmt.Sprintf("qa_%d", iteration)
	checkpoint, _, err := qualityLoopQACheckpointDiffHash(probe)
	return checkpoint, err
}

// qualityLoopArchiveInvariantSnapshot locks implementation and sealed evidence through archive.
// Proposal files are deliberately excluded because oz archive may rewrite their test references
// while moving them into the dated archive directory.
func qualityLoopArchiveInvariantSnapshot(repo string, state State) (string, error) {
	content, err := gitChangeContentSnapshotForChange(repo, state.ChangeName)
	if err != nil {
		return "", err
	}
	payloadDir, err := qualityLoopArchivePayloadDir(repo, state.ChangeName)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(repo, payloadDir)
	if err != nil {
		return "", err
	}
	prefix := strings.TrimPrefix(filepath.ToSlash(relative), "./")
	paths := gitContentSnapshotPathMap(content)
	entries := make([]string, 0, len(paths))
	for path, fingerprint := range paths {
		if path == prefix || strings.HasPrefix(path, strings.TrimSuffix(prefix, "/")+"/") {
			continue
		}
		entries = append(entries, path+"\x00"+fingerprint)
	}
	payloadEntries, err := qualityLoopArchivePayloadEntries(repo, payloadDir, state.ChangeName)
	if err != nil {
		return "", err
	}
	if len(payloadEntries) == 0 {
		return "", fmt.Errorf("archive 门禁找不到提案内容：%s", payloadDir)
	}
	sort.Strings(entries)
	sourceHash := qualityHashStrings(entries...)
	checkpoint, err := qualityLoopArchiveCheckpointStage(state)
	if err != nil {
		return "", err
	}
	acceptanceResult, err := verifyQualityAcceptanceCheckpoint(repo, state, checkpoint)
	if err != nil {
		return "", err
	}
	evidenceHash := acceptanceResult.EvidenceHash
	if state.QualityLoop.EvidenceHash == "" || evidenceHash != state.QualityLoop.EvidenceHash {
		return "", fmt.Errorf("archive 门禁 required evidence 已偏离最近通过的 acceptance")
	}
	return qualityHashStrings(sourceHash, evidenceHash), nil
}

var qualityLoopArchiveRelativeTestsReference = regexp.MustCompile(`(^|[[:space:]\x60"'(\[：])tests/`)

// qualityLoopArchivePayloadEntries hashes one proposal while normalizing deterministic archive rewrites.
func qualityLoopArchivePayloadEntries(repo, payloadDir, changeName string) ([]string, error) {
	relativePayload, err := filepath.Rel(repo, payloadDir)
	if err != nil {
		return nil, err
	}
	payloadPrefix := strings.TrimPrefix(filepath.ToSlash(relativePayload), "./") + "/"
	activePrefix := "docs/changes/" + changeName + "/"
	var entries []string
	err = filepath.WalkDir(payloadDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(payloadDir, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if utf8.Valid(data) {
			text := strings.ReplaceAll(string(data), payloadPrefix, "@current-change/")
			text = strings.ReplaceAll(text, activePrefix, "@current-change/")
			if relative != "tests" && !strings.HasPrefix(relative, "tests/") {
				text = normalizeQualityLoopArchiveRelativeTests(text)
			}
			data = []byte(text)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		fingerprint := qualityHashStrings(
			info.Mode().Type().String(),
			fmt.Sprintf("%t", info.Mode().Perm()&0o111 != 0),
			string(data),
		)
		entries = append(entries, "@current-change/"+relative+"\x00"+fingerprint)
		return nil
	})
	return entries, err
}

// normalizeQualityLoopArchiveRelativeTests canonicalizes the CLI's proposal-local tests rewrite.
func normalizeQualityLoopArchiveRelativeTests(text string) string {
	var out strings.Builder
	last := 0
	for _, match := range qualityLoopArchiveRelativeTestsReference.FindAllStringIndex(text, -1) {
		start, end := match[0], match[1]
		out.WriteString(text[last:start])
		if strings.HasPrefix(text[end:], "specs/") {
			out.WriteString(text[start:end])
		} else {
			out.WriteString(text[start : end-len("tests/")])
			out.WriteString("@current-change/tests/")
		}
		last = end
	}
	out.WriteString(text[last:])
	return out.String()
}

// qualityLoopArchivePayloadDir resolves the proposal before or after its archive move.
func qualityLoopArchivePayloadDir(repo, changeName string) (string, error) {
	active := filepath.Join(repo, "docs", "changes", changeName)
	if info, err := os.Stat(active); err == nil && info.IsDir() {
		return active, nil
	}
	matches, err := filepath.Glob(filepath.Join(repo, "docs", "changes", "archive", "*-"+changeName))
	if err != nil {
		return "", err
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return "", fmt.Errorf("archive 门禁找不到活动或归档提案 %q", changeName)
	}
	return matches[len(matches)-1], nil
}

// blockQualityLoopArchiveReadOnly records a recoverable archive boundary violation.
func (e *Engine) blockQualityLoopArchiveReadOnly(state *State, reason string) error {
	if state == nil {
		return fmt.Errorf("archive 只读门禁缺少 state")
	}
	stage := state.Stage
	if state.ArtifactGates == nil {
		state.ArtifactGates = map[string]StageValidationState{}
	}
	if state.Stages == nil {
		state.Stages = map[string]string{}
	}
	gate, err := reserveValidationAttempt(
		e.Repo,
		state,
		state.ArtifactGates[stage],
		validationKindArchiveReadOnly,
		func(reserved StageValidationState) {
			state.ArtifactGates[stage] = reserved
		},
	)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	attempt := ValidationAttempt{
		Stage:      stage,
		Kind:       validationKindArchiveReadOnly,
		Attempt:    gate.Attempts,
		Status:     validationStatusFailed,
		StartedAt:  now,
		FinishedAt: now,
		Commands: []ValidationCommandResult{{
			Command:  "oz flow archive read-only gate",
			ExitCode: 1,
			Output:   reason,
		}},
	}
	artifactPath, err := writeValidationAttempt(e.Repo, state.RunID, attempt)
	if err != nil {
		return err
	}
	gate.Kind = validationKindArchiveReadOnly
	gate.Status = validationStatusFailed
	gate.LastArtifact = artifactPath
	gate.LastError = reason
	state.ArtifactGates[stage] = gate
	currentHash, hashErr := e.currentQualityLoopDiffHash(*state)
	if hashErr != nil {
		return hashErr
	}
	probe := *state
	probe.QualityLoop.DiffHash = currentHash
	state.QualityLoop.GateFailureFingerprint = qualityHashStrings(validationKindArchiveReadOnly, reason)
	state.QualityLoop.GateProgressHash = qualityProgressHash(probe)
	state.QualityLoop.BlockedFromStage = stage
	state.Stages[stage] = statusBlockedStalled
	state.Status = statusBlockedStalled
	state.Stage = statusBlockedStalled
	state.Error = reason
	return nil
}

// rerouteQualityLoopArchiveRepair lets the repairer fix deterministic evidence packaging failures before a fresh QA.
func (e *Engine) rerouteQualityLoopArchiveRepair(state *State, reason string) error {
	if state == nil {
		return fmt.Errorf("archive 证据修复缺少 state")
	}
	fingerprint := qualityHashStrings(validationKindArchiveRepair, strings.TrimSpace(reason))
	if state.QualityLoop.ArchiveGateFingerprint == fingerprint {
		return e.blockQualityLoopArchiveReadOnly(state, reason)
	}
	archiveStage := state.Stage
	if err := e.blockQualityLoopArchiveReadOnly(state, reason); err != nil {
		return err
	}
	failedGate := state.ArtifactGates[archiveStage]
	nextStage := qualityLoopResumeAuditStage(state)
	failedGate.Kind = validationKindArchiveRepair
	state.ArtifactGates[nextStage] = failedGate
	state.Stages[archiveStage] = "rerouted"
	state.Status = statusRunning
	state.Stage = nextStage
	state.Error = ""
	state.QualityLoop.BlockedFromStage = ""
	state.QualityLoop.ResumeRerunPending = true
	state.QualityLoop.ArchiveGateFingerprint = fingerprint
	return nil
}

// isQualityLoopArchiveStage reports whether final delivery is moving an accepted proposal.
func isQualityLoopArchiveStage(state State) bool {
	return usesQualityLoop(state.Workflow) && state.Stage == workflowStageArchive
}

// blockQualityLoopQAReadOnly persists an observable, recoverable stop at the exact QA stage.
func (e *Engine) blockQualityLoopQAReadOnly(state *State, currentHash, reason string) error {
	if state == nil {
		return fmt.Errorf("QA 只读门禁缺少 state")
	}
	stage := state.Stage
	if state.ArtifactGates == nil {
		state.ArtifactGates = map[string]StageValidationState{}
	}
	if state.Stages == nil {
		state.Stages = map[string]string{}
	}
	gate, err := reserveValidationAttempt(
		e.Repo,
		state,
		state.ArtifactGates[stage],
		validationKindQAReadOnly,
		func(reserved StageValidationState) {
			state.ArtifactGates[stage] = reserved
		},
	)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	attempt := ValidationAttempt{
		Stage:      stage,
		Kind:       validationKindQAReadOnly,
		Attempt:    gate.Attempts,
		Status:     validationStatusFailed,
		StartedAt:  now,
		FinishedAt: now,
		Commands: []ValidationCommandResult{{
			Command:  "oz flow qa read-only gate",
			ExitCode: 1,
			Output:   reason,
		}},
	}
	artifactPath, err := writeValidationAttempt(e.Repo, state.RunID, attempt)
	if err != nil {
		return err
	}
	gate.Kind = validationKindQAReadOnly
	gate.Status = validationStatusFailed
	gate.LastArtifact = artifactPath
	gate.LastError = reason
	gate.DiffHash = currentHash
	state.ArtifactGates[stage] = gate
	state.Stages[stage] = statusBlockedStalled
	probe := *state
	probe.QualityLoop.DiffHash = currentHash
	state.QualityLoop.GateFailureFingerprint = qualityHashStrings(validationKindQAReadOnly, reason)
	state.QualityLoop.GateProgressHash = qualityProgressHash(probe)
	state.QualityLoop.BlockedFromStage = stage
	state.Status = statusBlockedStalled
	state.Stage = statusBlockedStalled
	state.Error = reason
	return nil
}

// isQualityLoopQAStage reports whether the current dynamic stage is independently reviewing code.
func isQualityLoopQAStage(state State) bool {
	if !usesQualityLoop(state.Workflow) {
		return false
	}
	stage, err := parseWorkflowStage(state.Stage)
	return err == nil && stage.isKind(workflowStageQA)
}

// bindQualityStageGateSnapshot rejects tests that changed the tracked source snapshot they validate.
func (e *Engine) bindQualityStageGateSnapshot(state *State) (bool, error) {
	if state == nil || !isQualityLoopRepairStage(*state) {
		return true, nil
	}
	content, err := gitChangeContentSnapshotForChange(e.Repo, state.ChangeName)
	if err != nil {
		return false, err
	}
	currentHash := qualityHashStrings(content)
	if currentHash != state.QualityLoop.DiffHash {
		if err := e.absorbQualityGateSourceSnapshot(state); err != nil {
			return false, err
		}
		current := state.AcceptanceRun[state.Stage]
		current.Kind = acceptanceRunKind
		current.Status = validationStatusFailed
		current.LastError = "deterministic gates changed the tracked source snapshot; rerun against the new diff"
		current.DiffHash = currentHash
		state.AcceptanceRun[state.Stage] = current
		recordQualityGateFailure(state, "diff_binding", current.LastError)
		if normalizeRunStatus(state.Status).isRunning() {
			state.Stages[state.Stage] = "validation_failed"
		}
		return false, nil
	}
	validation := state.Validation[state.Stage]
	validation.DiffHash = currentHash
	state.Validation[state.Stage] = validation
	acceptance := state.AcceptanceRun[state.Stage]
	acceptance.DiffHash = currentHash
	state.AcceptanceRun[state.Stage] = acceptance
	return true, nil
}

// absorbQualityGateSourceSnapshot prevents deterministic gate writes from looking like manual intervention.
func (e *Engine) absorbQualityGateSourceSnapshot(state *State) error {
	head, diff, err := gitSnapshot(e.Repo)
	if err != nil {
		return err
	}
	state.BaselineHead = head
	state.BaselineDiff = diff
	return nil
}

// shouldAdvanceAfterMainStage preserves loop and DAG scheduling boundaries.
func shouldAdvanceAfterMainStage(state State, mode stageGatePipelineMode) bool {
	if mode == stageGatePipelineLoop {
		return true
	}
	stage, err := parseWorkflowStage(state.Stage)
	if err != nil {
		return false
	}
	return stage.isKind(workflowStageExecution) || stage.isKind(workflowStageRepair) || stage.isKind(workflowStageAudit) || stage.isKind(workflowStageTargetedRepair) || stage.isKind(workflowStageFix) || stage.isKind(workflowStageArchive)
}

// failedGateProgressLabel maps persisted gate status to the existing progress vocabulary.
func failedGateProgressLabel(state State) string {
	if normalizeRunStatus(state.Status).isBlocked() {
		return "blocked"
	}
	return "validation_failed"
}

// nodeStageGateError converts a pipeline stop into the node contract's error style.
func nodeStageGateError(stage string, result stageGatePipelineResult) error {
	if !result.Done {
		return fmt.Errorf("%s 阶段 artifact 未完成", stage)
	}
	if result.Blocked {
		if result.ProgressLabel == "blocked" {
			return fmt.Errorf("%s gate blocked", stage)
		}
		return fmt.Errorf("%s validation 未通过", stage)
	}
	return nil
}
