// Package app tests the workflow stage decision layer extracted from Engine IO.
package app

import (
	"encoding/json"
	"testing"
)

// TestRepairSessionReuse verifies the clean repair path and records the durable session boundary.
func TestRepairSessionReuse(t *testing.T) {
	state := stageDecisionState("execution", 2)
	state.Sessions = map[string]string{
		sessionStateKey("codex", "repairer"): "thread-repairer",
		sessionStateKey("codex", "qa"):       "thread-qa",
	}
	state.Stages = map[string]string{
		"execution": "completed",
		"repair_1":  "completed",
		"qa_1":      "completed",
	}
	assertStageDecision(t, state, Review{}, QA{}, "repair_1", statusRunning, "")

	state.Stage = "repair_1"
	assertStageDecision(t, state, cleanReviewForStageDecision(), QA{}, "qa_1", statusRunning, "")

	state.Stage = "qa_1"
	assertStageDecision(t, state, Review{}, cleanQAForStageDecision(), "archive", statusRunning, "")

	state.Stage = "archive"
	assertStageDecision(t, state, Review{}, QA{}, "done", statusDone, "")

}

// TestZeroRepairStillRequiresQA verifies that disabling repair never grants archive authority.
func TestZeroRepairStillRequiresQA(t *testing.T) {
	state := stageDecisionState("execution", 0)
	assertStageDecision(t, state, Review{}, QA{}, "qa_1", statusRunning, "")

	state.Stage = "qa_1"
	assertStageDecision(t, state, Review{}, cleanQAForStageDecision(), "archive", statusRunning, "")

	qa := cleanQAForStageDecision()
	qa.Decision = "needs_fix"
	qa.Findings = []Finding{blockingFindingForStageDecision()}
	assertStageDecision(t, state, Review{}, qa, statusBlocked, statusBlocked, "未配置优化轮次")
}

// TestLegacyZeroReviewSnapshotResumesToArchive verifies an unversioned zero-round snapshot keeps its historical transition.
func TestLegacyZeroReviewSnapshotResumesToArchive(t *testing.T) {
	var state State
	if err := json.Unmarshal([]byte(`{
		"status":"running",
		"stage":"execution",
		"workflow_config":{"max_review_iterations":0,"stages":{"execution":{},"archive":{}}}
	}`), &state); err != nil {
		t.Fatal(err)
	}
	assertStageDecision(t, state, Review{}, QA{}, "archive", statusRunning, "")
}

// TestQARepairLoop verifies a failed independent QA enters the next repair round.
func TestQARepairLoop(t *testing.T) {
	state := stageDecisionState("qa_2", 3)
	qa := cleanQAForStageDecision()
	qa.Decision = "needs_fix"
	qa.Findings = []Finding{blockingFindingForStageDecision()}

	assertStageDecision(t, state, Review{}, qa, "repair_3", statusRunning, "")
}

// TestRepairLimitBlocks verifies the repair limit blocks instead of creating another round.
func TestRepairLimitBlocks(t *testing.T) {
	state := stageDecisionState("repair_3", 3)
	repair := cleanReviewForStageDecision()
	repair.Decision = "needs_more"
	repair.Findings = []Finding{blockingFindingForStageDecision()}
	assertStageDecision(t, state, repair, QA{}, statusBlocked, statusBlocked, "优化达到上限")
}

// TestWorkflowFailureReviewFailsWorkflow verifies reviewer-declared workflow failure ends the run.
func TestWorkflowFailureReviewFailsWorkflow(t *testing.T) {
	state := legacyStageDecisionState("review_1", 3)
	review := cleanReviewForStageDecision()
	review.Decision = "needs_fix"
	review.Findings = []Finding{blockingFindingForStageDecision()}
	review.WorkflowFailure = &ReviewWorkflowFailure{Failed: true, Reason: "acceptance contract is impossible"}

	assertStageDecision(t, state, review, QA{}, "review_1", statusFailed, "acceptance contract is impossible")
}

// TestStageDecisionReviewNeedsFix verifies review findings route to the matching fix stage.
func TestStageDecisionReviewNeedsFix(t *testing.T) {
	state := legacyStageDecisionState("review_2", 3)
	review := cleanReviewForStageDecision()
	review.Decision = "needs_fix"
	review.Findings = []Finding{blockingFindingForStageDecision()}

	assertStageDecision(t, state, review, QA{}, "fix_2", statusRunning, "")
}

// stageDecisionState returns the minimal durable state needed by the pure decision function.
func stageDecisionState(stage string, maxReviewIterations int) State {
	workflow := DefaultWorkflowConfig()
	workflow.MaxRepairIterations = maxReviewIterations
	workflow.MaxReviewIterations = 0
	return State{Status: statusRunning, Stage: stage, Workflow: workflow}
}

// legacyStageDecisionState returns a sealed review/fix snapshot for migration regression coverage.
func legacyStageDecisionState(stage string, maxReviewIterations int) State {
	workflow := DefaultWorkflowConfig()
	workflow.MaxRepairIterations = 0
	workflow.MaxReviewIterations = maxReviewIterations
	return State{Status: statusRunning, Stage: stage, Workflow: workflow}
}

// TestLegacyRepairSnapshotResume verifies old sealed review/fix stages keep their original transitions.
func TestLegacyRepairSnapshotResume(t *testing.T) {
	state := legacyStageDecisionState("execution", 2)
	assertStageDecision(t, state, Review{}, QA{}, "review_1", statusRunning, "")
	state.Stage = "fix_1"
	assertStageDecision(t, state, Review{}, QA{}, "review_2", statusRunning, "")
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
