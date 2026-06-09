// Package app tests review JSON validation for sealed workflow decisions.
package app

import (
	"os"
	"path/filepath"
	"testing"
)

// TestValidateReview accepts clean and fix review decisions.
func TestValidateReview(t *testing.T) {
	clean := Review{
		Summary:  "ok",
		Decision: "clean",
		Checks: ReviewChecks{
			OzAligned:                true,
			TasksVerified:            true,
			TestsMeaningful:          true,
			ImplementationScoped:     true,
			RuntimeBehaviorVerified:  true,
			PreviousFindingsResolved: true,
		},
		Evidence: []string{
			"validation artifact passed: validation-execution-1.json",
			"runtime evidence: Playwright screenshot test-results/demo.png",
		},
		Findings: []Finding{},
	}
	if err := ValidateReview(clean); err != nil {
		t.Fatal(err)
	}
	fix := Review{
		Summary:  "fix needed",
		Decision: "needs_fix",
		Findings: []Finding{{
			Title:          "bug",
			Severity:       "high",
			Evidence:       "test failed",
			Recommendation: "fix it",
		}},
	}
	if err := ValidateReview(fix); err != nil {
		t.Fatal(err)
	}
	if !NeedsFix(fix) {
		t.Fatal("fix review should require fix")
	}
}

// TestValidateReviewRejectsInvalidSchema rejects malformed reviewer output.
func TestValidateReviewRejectsInvalidSchema(t *testing.T) {
	bad := []Review{
		{Summary: "", Decision: "clean"},
		{Summary: "bad", Decision: "maybe"},
		{Summary: "bad", Decision: "clean", Findings: []Finding{{Title: "unexpected"}}},
		{Summary: "bad", Decision: "clean", Findings: []Finding{}},
		{Summary: "bad", Decision: "needs_fix", Findings: []Finding{}},
		{Summary: "bad", Decision: "needs_fix", Findings: []Finding{{Title: "bug", Severity: "urgent"}}},
	}
	for _, review := range bad {
		if err := ValidateReview(review); err == nil {
			t.Fatalf("expected invalid review: %#v", review)
		}
	}
}

// TestReadReviewNormalizesSeverityAliases verifies agent severity vocabulary is stored canonically.
func TestReadReviewNormalizesSeverityAliases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review-1.json")
	body := `{"summary":"fix","decision":"needs_fix","checks":{},"evidence":[],"findings":[{"title":"bug","severity":"high","evidence":"test failed","recommendation":"fix it"}]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	review, err := ReadReview(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := review.Findings[0].Severity; got != "major" {
		t.Fatalf("severity = %q, want major", got)
	}
}

// TestValidateReviewAcceptsWorkflowFailure verifies strict JSON can stop futile loops.
func TestValidateReviewAcceptsWorkflowFailure(t *testing.T) {
	review := Review{
		Summary:  "no progress",
		Decision: "needs_fix",
		WorkflowFailure: &ReviewWorkflowFailure{
			Failed: true,
			Reason: "连续两轮没有实质变化，且 fix summary 表示缺少必要凭据",
		},
		Findings: []Finding{{
			Title:          "cannot complete",
			Severity:       "blocker",
			Evidence:       "diff unchanged and fix summary reports missing credentials",
			Recommendation: "stop workflow",
		}},
	}
	if err := ValidateReviewForIteration(review, 2); err != nil {
		t.Fatal(err)
	}
	if !ReviewDeclaresWorkflowFailure(review) {
		t.Fatal("review should declare workflow failure")
	}
}

// TestValidateReviewRejectsInvalidWorkflowFailure verifies failure fields stay strict.
func TestValidateReviewRejectsInvalidWorkflowFailure(t *testing.T) {
	for _, review := range []Review{
		{Summary: "bad", Decision: "clean", WorkflowFailure: &ReviewWorkflowFailure{Failed: true, Reason: "stop"}},
		{Summary: "bad", Decision: "needs_fix", WorkflowFailure: &ReviewWorkflowFailure{Failed: true}, Findings: []Finding{{Title: "bug", Severity: "major", Evidence: "e", Recommendation: "r"}}},
	} {
		if err := ValidateReviewForIteration(review, 2); err == nil {
			t.Fatalf("expected invalid workflow failure: %#v", review)
		}
	}
}

// TestValidateReviewForIterationRequiresHistoryResolution verifies later clean reviews prove fixes.
func TestValidateReviewForIterationRequiresHistoryResolution(t *testing.T) {
	review := Review{
		Summary:  "ok",
		Decision: "clean",
		Checks: ReviewChecks{
			OzAligned:               true,
			TasksVerified:           true,
			TestsMeaningful:         true,
			ImplementationScoped:    true,
			RuntimeBehaviorVerified: true,
		},
		Evidence: []string{
			"validation artifact passed: validation-execution-1.json",
			"runtime evidence: Playwright screenshot test-results/demo.png",
		},
		Findings: []Finding{},
	}
	if err := ValidateReviewForIteration(review, 1); err == nil {
		t.Fatal("clean review should require previous_findings_resolved")
	}
	review.Checks.PreviousFindingsResolved = true
	if err := ValidateReviewForIteration(review, 2); err != nil {
		t.Fatal(err)
	}
}
