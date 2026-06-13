// Package app tests the workflow stage decision layer extracted from Engine IO.
package app

import "testing"

// TestEngineStartRunsCleanReviewsToDone verifies the clean path reaches archive after execution, review, and QA.
func TestEngineStartRunsCleanReviewsToDone(t *testing.T) {
	state := stageDecisionState("execution", 2)
	assertStageDecision(t, state, Review{}, QA{}, "review_1", statusRunning, "")

	state.Stage = "review_1"
	assertStageDecision(t, state, cleanReviewForStageDecision(), QA{}, "qa_1", statusRunning, "")

	state.Stage = "qa_1"
	assertStageDecision(t, state, Review{}, cleanQAForStageDecision(), "archive", statusRunning, "")

	state.Stage = "archive"
	assertStageDecision(t, state, Review{}, QA{}, "done", statusDone, "")
}

// TestQAFailureReturnsToFix verifies a failed QA round routes back to the matching fix stage.
func TestQAFailureReturnsToFix(t *testing.T) {
	state := stageDecisionState("qa_2", 3)
	qa := cleanQAForStageDecision()
	qa.Decision = "needs_fix"
	qa.Findings = []Finding{blockingFindingForStageDecision()}

	assertStageDecision(t, state, Review{}, qa, "fix_2", statusRunning, "")
}

// TestEngineBlocksAfterLastFix verifies the review limit blocks instead of creating another review round.
func TestEngineBlocksAfterLastFix(t *testing.T) {
	state := stageDecisionState("fix_3", 3)
	assertStageDecision(t, state, Review{}, QA{}, statusBlocked, statusBlocked, "审核修正达到上限")
}

// TestWorkflowFailureReviewFailsWorkflow verifies reviewer-declared workflow failure ends the run.
func TestWorkflowFailureReviewFailsWorkflow(t *testing.T) {
	state := stageDecisionState("review_1", 3)
	review := cleanReviewForStageDecision()
	review.Decision = "needs_fix"
	review.Findings = []Finding{blockingFindingForStageDecision()}
	review.WorkflowFailure = &ReviewWorkflowFailure{Failed: true, Reason: "acceptance contract is impossible"}

	assertStageDecision(t, state, review, QA{}, "review_1", statusFailed, "acceptance contract is impossible")
}

// TestStageDecisionReviewNeedsFix verifies review findings route to the matching fix stage.
func TestStageDecisionReviewNeedsFix(t *testing.T) {
	state := stageDecisionState("review_2", 3)
	review := cleanReviewForStageDecision()
	review.Decision = "needs_fix"
	review.Findings = []Finding{blockingFindingForStageDecision()}

	assertStageDecision(t, state, review, QA{}, "fix_2", statusRunning, "")
}

// stageDecisionState returns the minimal durable state needed by the pure decision function.
func stageDecisionState(stage string, maxReviewIterations int) State {
	workflow := DefaultWorkflowConfig()
	workflow.MaxReviewIterations = maxReviewIterations
	return State{Status: statusRunning, Stage: stage, Workflow: workflow}
}

// assertStageDecision checks the business-level next stage, status, and blocking reason.
func assertStageDecision(t *testing.T, state State, review Review, qa QA, wantStage, wantStatus, wantReason string) {
	t.Helper()
	decision, err := DecideNextStage(state, review, qa)
	if err != nil {
		t.Fatal(err)
	}
	if decision.NextStage != wantStage {
		t.Fatalf("NextStage = %q, want %q", decision.NextStage, wantStage)
	}
	if decision.NextStatus != wantStatus {
		t.Fatalf("NextStatus = %q, want %q", decision.NextStatus, wantStatus)
	}
	if wantReason != "" && !containsStageDecisionText(decision.BlockedReason, wantReason) {
		t.Fatalf("BlockedReason = %q, want containing %q", decision.BlockedReason, wantReason)
	}
	if wantReason == "" && decision.BlockedReason != "" {
		t.Fatalf("BlockedReason = %q, want empty", decision.BlockedReason)
	}
}

// cleanReviewForStageDecision returns a review artifact that represents a clean review decision.
func cleanReviewForStageDecision() Review {
	return Review{
		Summary:  "clean",
		Decision: "clean",
		Checks: ReviewChecks{
			OzAligned:                true,
			TasksVerified:            true,
			TestsMeaningful:          true,
			ImplementationScoped:     true,
			RuntimeBehaviorVerified:  true,
			PreviousFindingsResolved: true,
		},
		Evidence: []string{"go test ./internal/app"},
	}
}

// cleanQAForStageDecision returns a QA artifact that represents a clean QA decision.
func cleanQAForStageDecision() QA {
	return QA{
		Summary:  "clean",
		Decision: "clean",
		Evidence: []string{"go test ./internal/app"},
	}
}

// blockingFindingForStageDecision returns a current-change finding that must trigger a fix.
func blockingFindingForStageDecision() Finding {
	return Finding{
		Title:          "regression",
		Severity:       "major",
		Evidence:       "stage decision test",
		Recommendation: "fix the transition",
		Scope:          findingScopeCurrentChange,
	}
}

// containsStageDecisionText keeps the tests independent from exact localized prefixes.
func containsStageDecisionText(got, want string) bool {
	for i := 0; i+len(want) <= len(got); i++ {
		if got[i:i+len(want)] == want {
			return true
		}
	}
	return want == ""
}
