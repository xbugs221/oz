// Package app decides workflow stage completion and progression for sealed runs.
package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// StageDecision describes the durable mutation needed after a workflow stage completes.
type StageDecision struct {
	NextStage                 string
	NextStatus                string
	BlockedReason             string
	NeedsRerun                bool
	UpdateRepairConfirmation  bool
	RepairConfirmationPending bool
	QualityLoop               *QualityLoopState
}

// DecideNextStage returns the next durable stage/status for pure stage transitions.
func DecideNextStage(state State, review Review, qa QA) (StageDecision, error) {
	ensureWorkflowConfig(&state)
	stage, err := parseWorkflowStage(state.Stage)
	if err != nil {
		return StageDecision{}, err
	}
	if usesQualityLoop(state.Workflow) {
		return decideQualityLoopStage(state, stage, review, qa)
	}
	switch stage.Kind {
	case workflowStageExecution:
		if usesRepairWorkflow(state.Workflow) && state.Workflow.MaxRepairIterations == 0 {
			return StageDecision{NextStage: "qa_1", NextStatus: state.Status}, nil
		}
		if state.Workflow.MaxRepairIterations > 0 {
			return StageDecision{NextStage: "repair_1", NextStatus: state.Status}, nil
		}
		if state.Workflow.MaxReviewIterations == 0 {
			return StageDecision{NextStage: "archive", NextStatus: state.Status}, nil
		}
		return StageDecision{NextStage: "review_1", NextStatus: state.Status}, nil
	case workflowStageRepair:
		if RepairNeedsMore(review) {
			if stage.Iteration >= state.Workflow.MaxRepairIterations {
				return StageDecision{NextStage: statusBlocked, NextStatus: statusBlocked, BlockedReason: "优化达到上限，工作流已中断"}, nil
			}
			return StageDecision{
				NextStage: fmt.Sprintf("repair_%d", stage.Iteration+1), NextStatus: state.Status,
				UpdateRepairConfirmation: true,
			}, nil
		}
		if state.RepairConfirmationPending {
			return StageDecision{
				NextStage: fmt.Sprintf("qa_%d", stage.Iteration), NextStatus: state.Status,
				UpdateRepairConfirmation: true,
			}, nil
		}
		if stage.Iteration >= state.Workflow.MaxRepairIterations {
			return StageDecision{
				NextStage: statusBlocked, NextStatus: statusBlocked,
				BlockedReason: "优化已完成但达到上限，缺少最终重审确认",
			}, nil
		}
		return StageDecision{
			NextStage: fmt.Sprintf("repair_%d", stage.Iteration+1), NextStatus: state.Status,
			UpdateRepairConfirmation: true, RepairConfirmationPending: true,
		}, nil
	case workflowStageReview:
		n := strconv.Itoa(stage.Iteration)
		if ReviewDeclaresWorkflowFailure(review) {
			reason := "审核阶段判定工作流无法继续：" + strings.TrimSpace(review.WorkflowFailure.Reason)
			return StageDecision{NextStage: state.Stage, NextStatus: statusFailed, BlockedReason: reason}, nil
		}
		if NeedsFix(review) {
			return StageDecision{NextStage: "fix_" + n, NextStatus: state.Status}, nil
		}
		return StageDecision{NextStage: "qa_" + n, NextStatus: state.Status}, nil
	case workflowStageQA:
		n := strconv.Itoa(stage.Iteration)
		if QANeedsFix(qa) {
			if state.Workflow.MaxRepairIterations > 0 {
				if stage.Iteration >= state.Workflow.MaxRepairIterations {
					return StageDecision{NextStage: statusBlocked, NextStatus: statusBlocked, BlockedReason: "独立 QA 未通过且优化达到上限，工作流已中断"}, nil
				}
				return StageDecision{
					NextStage: fmt.Sprintf("repair_%d", stage.Iteration+1), NextStatus: state.Status,
					UpdateRepairConfirmation: true,
				}, nil
			}
			if usesRepairWorkflow(state.Workflow) {
				return StageDecision{NextStage: statusBlocked, NextStatus: statusBlocked, BlockedReason: "独立 QA 未通过且未配置优化轮次，工作流已中断"}, nil
			}
			return StageDecision{NextStage: "fix_" + n, NextStatus: state.Status}, nil
		}
		return StageDecision{NextStage: "archive", NextStatus: state.Status}, nil
	case workflowStageFix:
		if stage.Iteration >= state.Workflow.MaxReviewIterations {
			return StageDecision{NextStage: statusBlocked, NextStatus: statusBlocked, BlockedReason: "审核修正达到上限，工作流已中断"}, nil
		}
		return StageDecision{NextStage: fmt.Sprintf("review_%d", stage.Iteration+1), NextStatus: state.Status}, nil
	case workflowStageArchive:
		return StageDecision{NextStage: "done", NextStatus: statusDone}, nil
	default:
		return StageDecision{}, fmt.Errorf("未知阶段 %q", state.Stage)
	}
}

// decideQualityLoopStage drives new runs by quality outcomes instead of a round budget.
func decideQualityLoopStage(state State, stage workflowStage, repair Review, qa QA) (StageDecision, error) {
	nextStatus := state.Status
	if nextStatus == "" {
		nextStatus = statusRunning
	}
	switch stage.Kind {
	case workflowStageExecution:
		quality := state.QualityLoop
		quality.Mode = "pre_qa_audit"
		return StageDecision{NextStage: "audit_1", NextStatus: nextStatus, QualityLoop: &quality}, nil
	case workflowStageAudit:
		quality := state.QualityLoop
		quality.Mode = "pre_qa_audit"
		quality.SourceQAArtifact = ""
		quality.SourceQAHash = ""
		quality.RequiredTestsPassed = qualityStageTestsPassed(state, state.Stage)
		if RepairNeedsMore(repair) {
			fingerprint := reviewFindingFingerprint(repair)
			progress := qualityProgressHash(state)
			if quality.FindingFingerprint == fingerprint && quality.ProgressHash == progress {
				return qualityStalledDecision(state, quality), nil
			}
			quality.FindingFingerprint = fingerprint
			quality.ProgressHash = progress
			return StageDecision{NextStage: fmt.Sprintf("audit_%d", stage.Iteration+1), NextStatus: nextStatus, QualityLoop: &quality}, nil
		}
		quality.FindingFingerprint = ""
		quality.ProgressHash = ""
		return StageDecision{
			NextStage:   fmt.Sprintf("qa_%d", nextQualityLoopQAIteration(state)),
			NextStatus:  nextStatus,
			QualityLoop: &quality,
		}, nil
	case workflowStageQA:
		quality := state.QualityLoop
		quality.SourceQAArtifact = fmt.Sprintf("qa-%d.json", stage.Iteration)
		quality.SourceQAHash = qaArtifactContentHash(qa)
		if !QANeedsFix(qa) {
			quality.FindingFingerprint = ""
			return StageDecision{
				NextStage: workflowStageArchive, NextStatus: nextStatus, QualityLoop: &quality,
			}, nil
		}
		quality.Mode = "qa_targeted_repair"
		fingerprint := qaFindingFingerprint(qa)
		progress := qualityProgressHash(state)
		if quality.FindingFingerprint == fingerprint && quality.ProgressHash == progress {
			return qualityStalledDecision(state, quality), nil
		}
		quality.FindingFingerprint = fingerprint
		quality.ProgressHash = progress
		quality.RerunFindingFingerprint = ""
		quality.RerunProgressHash = ""
		quality.RequiredTestsPassed = false
		quality.FailedTestsReplayed = false
		return StageDecision{
			NextStage: fmt.Sprintf("targeted_repair_%d", stage.Iteration), NextStatus: nextStatus,
			QualityLoop: &quality,
		}, nil
	case workflowStageTargetedRepair:
		quality := state.QualityLoop
		quality.Mode = "qa_targeted_repair"
		quality.RequiredTestsPassed = qualityStageTestsPassed(state, state.Stage)
		quality.FailedTestsReplayed = quality.RequiredTestsPassed
		if RepairNeedsMore(repair) {
			fingerprint := reviewFindingFingerprint(repair)
			progress := qualityProgressHash(state)
			if quality.RerunFindingFingerprint == fingerprint && quality.RerunProgressHash == progress {
				return qualityStalledDecision(state, quality), nil
			}
			quality.RerunFindingFingerprint = fingerprint
			quality.RerunProgressHash = progress
			return StageDecision{
				NextStage: state.Stage, NextStatus: nextStatus, NeedsRerun: true, QualityLoop: &quality,
			}, nil
		}
		quality.RerunFindingFingerprint = ""
		quality.RerunProgressHash = ""
		return StageDecision{
			NextStage: fmt.Sprintf("qa_%d", stage.Iteration+1), NextStatus: nextStatus, QualityLoop: &quality,
		}, nil
	case workflowStageArchive:
		return StageDecision{NextStage: workflowStageDone, NextStatus: statusDone}, nil
	default:
		return StageDecision{}, fmt.Errorf("quality loop 未知阶段 %q", state.Stage)
	}
}

// nextQualityLoopQAIteration allocates a fresh QA artifact after any re-audit.
func nextQualityLoopQAIteration(state State) int {
	latest := 0
	for name := range state.Stages {
		stage, err := parseWorkflowStage(name)
		if err == nil && stage.isKind(workflowStageQA) && stage.Iteration > latest {
			latest = stage.Iteration
		}
	}
	return latest + 1
}

// qualityStageTestsPassed checks that required tests completed for the current source snapshot.
func qualityStageTestsPassed(state State, stage string) bool {
	run := state.AcceptanceRun[stage]
	return run.Status == validationStatusPassed
}

// qualityStalledDecision pauses the exact active stage until its progress inputs change.
func qualityStalledDecision(state State, quality QualityLoopState) StageDecision {
	quality.BlockedFromStage = state.Stage
	return StageDecision{
		NextStage: statusBlockedStalled, NextStatus: statusBlockedStalled,
		BlockedReason: "相同 findings 下源码、测试、验证与 evidence 均无变化；请提供新代码、证据、配置或人工指令后恢复",
		QualityLoop:   &quality,
	}
}

// isQualityLoopRepairStage limits unbounded quality-gate retries to dynamic repair stages.
func isQualityLoopRepairStage(state State) bool {
	if !usesQualityLoop(state.Workflow) {
		return false
	}
	stage, err := parseWorkflowStage(state.Stage)
	return err == nil && (stage.isKind(workflowStageAudit) || stage.isKind(workflowStageTargetedRepair))
}

// recordQualityGateFailure pauses adjacent identical gate failures only when no progress changed.
func recordQualityGateFailure(state *State, kind, failure string) bool {
	if state == nil || !isQualityLoopRepairStage(*state) {
		return false
	}
	fingerprint := qualityHashStrings(kind, strings.TrimSpace(failure))
	progress := qualityProgressHash(*state)
	if state.QualityLoop.GateFailureFingerprint == fingerprint &&
		state.QualityLoop.GateProgressHash == progress {
		quality := state.QualityLoop
		decision := qualityStalledDecision(*state, quality)
		state.QualityLoop = *decision.QualityLoop
		state.Status = decision.NextStatus
		state.Stage = decision.NextStage
		state.Error = decision.BlockedReason
		return true
	}
	state.QualityLoop.GateFailureFingerprint = fingerprint
	state.QualityLoop.GateProgressHash = progress
	return false
}

// clearQualityGateFailure forgets a transient gate baseline after every deterministic gate passes.
func clearQualityGateFailure(state *State) {
	if state == nil || !usesQualityLoop(state.Workflow) {
		return
	}
	state.QualityLoop.GateFailureFingerprint = ""
	state.QualityLoop.GateProgressHash = ""
}

// qaFindingFingerprint normalizes QA findings and failed acceptance IDs into one stable key.
func qaFindingFingerprint(qa QA) string {
	var failed []string
	for _, result := range qa.AcceptanceMatrix {
		if normalizeAcceptanceStatus(result.Status) != "passed" {
			failed = append(failed, result.ID)
		}
	}
	return findingFingerprint(qa.Findings, failed)
}

// reviewFindingFingerprint normalizes self-review findings for audit and rerun stalls.
func reviewFindingFingerprint(review Review) string {
	return findingFingerprint(review.Findings, nil)
}

// findingFingerprint makes finding order and cosmetic punctuation irrelevant to progress checks.
func findingFingerprint(findings []Finding, extra []string) string {
	parts := make([]string, 0, len(findings)+len(extra))
	for _, finding := range findings {
		parts = append(parts, fmt.Sprintf("%s\x00%s\x00%s", findingKey(finding.Title), finding.Severity, finding.Scope))
	}
	for _, item := range extra {
		parts = append(parts, "acceptance\x00"+strings.TrimSpace(item))
	}
	sort.Strings(parts)
	return qualityHashStrings(parts...)
}

// qualityProgressHash binds progress to source and semantic gate outcomes, not volatile logs.
func qualityProgressHash(state State) string {
	diffHash := state.QualityLoop.DiffHash
	if diffHash == "" {
		diffHash = qualityHashStrings(state.BaselineDiff)
	}
	testsHash := state.QualityLoop.TestsProgressHash
	if testsHash == "" {
		testsHash = state.QualityLoop.TestsHash
	}
	validationHash := state.QualityLoop.ValidationProgressHash
	if validationHash == "" {
		validationHash = state.QualityLoop.ValidationHash
	}
	evidenceHash := state.QualityLoop.EvidenceProgressHash
	if evidenceHash == "" {
		evidenceHash = state.QualityLoop.EvidenceHash
	}
	return qualityHashStrings(
		diffHash,
		testsHash,
		validationHash,
		evidenceHash,
	)
}

// qualityBlockedProgressHash selects the baseline that caused the active stalled block.
func qualityBlockedProgressHash(state State) string {
	if state.QualityLoop.GateFailureFingerprint != "" &&
		state.QualityLoop.GateProgressHash != "" {
		return state.QualityLoop.GateProgressHash
	}
	if strings.HasPrefix(state.QualityLoop.BlockedFromStage, "targeted_repair_") &&
		state.QualityLoop.RerunProgressHash != "" {
		return state.QualityLoop.RerunProgressHash
	}
	return state.QualityLoop.ProgressHash
}

// qualityHashStrings hashes stable progress components with unambiguous separators.
func qualityHashStrings(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		fmt.Fprintf(hash, "%s\x00", part)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// stageKind collapses iteration stages to their shared prompt and config kind.
func stageKind(stage string) string {
	role, err := roleForStage(stage)
	if err != nil {
		return stage
	}
	return role.Name
}

// stageIteration returns the numeric review, QA, or fix round encoded in the stage name.
func stageIteration(stage string) (int, error) {
	parsed, err := parseWorkflowStage(stage)
	if err != nil {
		return 0, err
	}
	if parsed.Iterable {
		return parsed.Iteration, nil
	}
	return 0, nil
}

type fixEscalationPlan struct {
	Enabled               bool
	ConsecutiveFailures   int
	Reasoning             string
	RepeatedFindingTitles []string
}

// fixEscalation reports whether a fix follows repeated failed reviews.
func fixEscalation(repo string, state State) (fixEscalationPlan, error) {
	if !strings.HasPrefix(state.Stage, "fix_") {
		return fixEscalationPlan{}, nil
	}
	iteration, err := stageIteration(state.Stage)
	if err != nil {
		return fixEscalationPlan{}, err
	}
	if iteration < 2 {
		return fixEscalationPlan{}, nil
	}
	reviews := make([]Review, 0, iteration)
	failures := 0
	for i := iteration; i >= 1; i-- {
		review, err := ReadReview(filepath.Join(runDir(repo, state.RunID), fmt.Sprintf("review-%d.json", i)))
		if os.IsNotExist(err) {
			break
		}
		if err != nil {
			return fixEscalationPlan{}, err
		}
		if !NeedsFix(review) {
			break
		}
		reviews = append(reviews, review)
		failures++
	}
	if failures < 2 {
		return fixEscalationPlan{}, nil
	}
	plan := fixEscalationPlan{
		Enabled:             true,
		ConsecutiveFailures: failures,
		Reasoning:           reasoningForConsecutiveFailures(failures),
	}
	if plan.Reasoning == "low" {
		return fixEscalationPlan{}, nil
	}
	if len(reviews) >= 2 {
		plan.RepeatedFindingTitles = repeatedFindingTitles(reviews[0], reviews[1])
	}
	return plan, nil
}

func reasoningForConsecutiveFailures(failures int) string {
	switch {
	case failures >= 4:
		return "xhigh"
	case failures >= 3:
		return "high"
	case failures >= 2:
		return "medium"
	default:
		return "low"
	}
}

func higherReasoning(current, target string) string {
	if reasoningRank(current) >= reasoningRank(target) {
		return current
	}
	return target
}

func reasoningRank(reasoning string) int {
	switch reasoning {
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	case "xhigh":
		return 4
	default:
		return 0
	}
}

// repeatedFindingTitles returns current finding titles that also appeared in the previous review.
func repeatedFindingTitles(current, previous Review) []string {
	seen := map[string]bool{}
	for _, finding := range previous.Findings {
		key := findingKey(finding.Title)
		if key != "" {
			seen[key] = true
		}
	}
	var repeated []string
	for _, finding := range current.Findings {
		key := findingKey(finding.Title)
		if key != "" && seen[key] {
			repeated = append(repeated, finding.Title)
		}
	}
	return repeated
}

func findingKey(title string) string {
	var out strings.Builder
	lastSpace := true
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			out.WriteRune(r)
			lastSpace = false
		case unicode.IsSpace(r) || r == '-' || r == '_' || r == ':' || r == '：':
			if !lastSpace {
				out.WriteByte(' ')
				lastSpace = true
			}
		}
	}
	return strings.TrimSpace(out.String())
}

// advance moves state to the next linear stage, honoring review fix decisions.
func (e *Engine) advance(state *State) error {
	ensureWorkflowConfig(state)
	result, done, err := e.validateStageArtifact(*state)
	if err != nil {
		return e.stageArtifactGateError(*state, err)
	}
	if !done {
		return e.stageArtifactGateError(*state, fmt.Errorf("%s 阶段 artifact 未完成", state.Stage))
	}
	review := result.Review
	if stage, parseErr := parseWorkflowStage(state.Stage); parseErr == nil {
		switch stage.Kind {
		case workflowStageRepair, workflowStageAudit, workflowStageTargetedRepair:
			review = result.Repair
		}
	}
	qa := result.QA
	stage, err := parseWorkflowStage(state.Stage)
	if err != nil {
		return err
	}
	switch stage.Kind {
	case workflowStageExecution:
	case workflowStageRepair:
		clearStageValidationFailure(state)
	case workflowStageAudit, workflowStageTargetedRepair:
		clearStageValidationFailure(state)
	case workflowStageReview:
		clearStageValidationFailure(state)
	case workflowStageQA:
		clearStageValidationFailure(state)
	case workflowStageFix:
	case workflowStageArchive:
	default:
		return fmt.Errorf("未知阶段 %q", state.Stage)
	}
	decision, err := DecideNextStage(*state, review, qa)
	if err != nil {
		return err
	}
	if decision.UpdateRepairConfirmation {
		state.RepairConfirmationPending = decision.RepairConfirmationPending
	}
	if decision.QualityLoop != nil {
		state.QualityLoop = *decision.QualityLoop
	}
	if decision.NeedsRerun {
		state.Stages[state.Stage] = "needs_more"
	}
	state.Stage = decision.NextStage
	state.Status = decision.NextStatus
	if decision.BlockedReason != "" {
		state.Error = decision.BlockedReason
	}
	return nil
}

// validateArchiveReadiness blocks archive completion until the evidence chain is complete.
func (e *Engine) validateArchiveReadiness(state State) error {
	if !fileExists(filepath.Join(runDir(e.Repo, state.RunID), "delivery-summary.md")) || !archiveExists(e.Repo, state.ChangeName) {
		return fmt.Errorf("archive 阶段缺少 delivery summary 或归档目录")
	}
	if err := e.verifyQualityLoopArchivedAcceptance(state); err != nil {
		return err
	}
	iteration := latestCompletedQAIteration(state)
	if iteration == 0 {
		return fmt.Errorf("archive 阶段缺少 clean QA artifact")
	}
	if usesRepairWorkflow(state.Workflow) && state.Workflow.MaxRepairIterations > 0 {
		repair, err := ReadRepair(filepath.Join(runDir(e.Repo, state.RunID), fmt.Sprintf("repair-%d.json", iteration)))
		if err != nil {
			return err
		}
		if RepairNeedsMore(repair) {
			return fmt.Errorf("archive 阶段发现 repair-%d 仍需继续", iteration)
		}
	} else if !usesQualityLoop(state.Workflow) && state.Workflow.MaxReviewIterations > 0 {
		review, err := ReadReview(filepath.Join(runDir(e.Repo, state.RunID), fmt.Sprintf("review-%d.json", iteration)))
		if err != nil {
			return err
		}
		if NeedsFix(review) {
			return fmt.Errorf("archive 阶段发现 review-%d 仍需修复", iteration)
		}
	}
	qa, err := ReadQA(filepath.Join(runDir(e.Repo, state.RunID), fmt.Sprintf("qa-%d.json", iteration)))
	if err != nil {
		return err
	}
	acceptance, err := readAcceptanceForState(e.Repo, state)
	if err != nil {
		return err
	}
	if err := ValidateQAAgainstAcceptance(qa, acceptance); err != nil {
		return err
	}
	if QANeedsFix(qa) {
		return fmt.Errorf("archive 阶段发现 qa-%d 仍需修复", iteration)
	}
	if len(state.Workflow.Validation.Commands) > 0 {
		return validateArchiveValidationEvidence(state)
	}
	return nil
}

// latestCompletedQAIteration derives the latest durable QA from stage records for dynamic runs.
func latestCompletedQAIteration(state State) int {
	latest := 0
	if usesQualityLoop(state.Workflow) {
		for name, status := range state.Stages {
			if status != "completed" || !strings.HasPrefix(name, "qa_") {
				continue
			}
			iteration, err := stageIteration(name)
			if err == nil && iteration > latest {
				latest = iteration
			}
		}
		return latest
	}
	limit := state.Workflow.MaxRepairIterations
	if limit == 0 {
		limit = state.Workflow.MaxReviewIterations
		if limit == 0 {
			limit = 1
		}
	}
	for i := 1; i <= limit; i++ {
		if state.Stages[fmt.Sprintf("qa_%d", i)] != "" {
			latest = i
		}
	}
	return latest
}

// validateArchiveValidationEvidence checks every implementation stage that required validation.
func validateArchiveValidationEvidence(state State) error {
	for stage, status := range state.Stages {
		if status == "" {
			continue
		}
		if stage != "execution" && !strings.HasPrefix(stage, "repair_") && !strings.HasPrefix(stage, "audit_") &&
			!strings.HasPrefix(stage, "targeted_repair_") && !strings.HasPrefix(stage, "fix_") {
			continue
		}
		if state.Validation[stage].Status != validationStatusPassed {
			return fmt.Errorf("archive 阶段缺少 %s 的 validation passed 记录", stage)
		}
	}
	return nil
}
