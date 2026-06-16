// Package app decides workflow stage completion and progression for sealed runs.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// StageDecision describes the durable mutation needed after a workflow stage completes.
type StageDecision struct {
	NextStage     string
	NextStatus    string
	BlockedReason string
	NeedsRerun    bool
}

// DecideNextStage returns the next durable stage/status for pure stage transitions.
func DecideNextStage(state State, review Review, qa QA) (StageDecision, error) {
	ensureWorkflowConfig(&state)
	stage, err := parseWorkflowStage(state.Stage)
	if err != nil {
		return StageDecision{}, err
	}
	switch stage.Kind {
	case workflowStageExecution:
		if state.Workflow.MaxReviewIterations == 0 {
			return StageDecision{NextStage: "archive", NextStatus: state.Status}, nil
		}
		return StageDecision{NextStage: "review_1", NextStatus: state.Status}, nil
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
	qa := result.QA
	stage, err := parseWorkflowStage(state.Stage)
	if err != nil {
		return err
	}
	switch stage.Kind {
	case workflowStageExecution:
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
	if state.Workflow.MaxReviewIterations == 0 {
		return nil
	}
	iteration := latestCompletedQAIteration(state)
	if iteration == 0 {
		return fmt.Errorf("archive 阶段缺少 clean QA artifact")
	}
	review, err := ReadReview(filepath.Join(runDir(e.Repo, state.RunID), fmt.Sprintf("review-%d.json", iteration)))
	if err != nil {
		return err
	}
	if NeedsFix(review) {
		return fmt.Errorf("archive 阶段发现 review-%d 仍需修复", iteration)
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

func latestCompletedQAIteration(state State) int {
	latest := 0
	for i := 1; i <= state.Workflow.MaxReviewIterations; i++ {
		if state.Stages[fmt.Sprintf("qa_%d", i)] != "" {
			latest = i
		}
	}
	return latest
}

func validateArchiveValidationEvidence(state State) error {
	for stage, status := range state.Stages {
		if status == "" {
			continue
		}
		if stage != "execution" && !strings.HasPrefix(stage, "fix_") {
			continue
		}
		if state.Validation[stage].Status != validationStatusPassed {
			return fmt.Errorf("archive 阶段缺少 %s 的 validation passed 记录", stage)
		}
	}
	return nil
}
