// Package app tests artifact gate retry errors and their wrapped causes.
package app

import (
	"errors"
	"strings"
	"testing"
)

// TestStageArtifactGateErrorWrapsOriginalFailure verifies callers can inspect the root failure.
func TestStageArtifactGateErrorWrapsOriginalFailure(t *testing.T) {
	original := errors.New("review schema failed")
	state := State{RunID: "run-gate", ChangeName: "demo", Stage: "review_2"}
	err := (&Engine{Repo: t.TempDir()}).stageArtifactGateError(state, original)

	if !isStageArtifactGateError(err) {
		t.Fatalf("error %T was not classified as stage artifact gate", err)
	}
	if !errors.Is(err, original) {
		t.Fatalf("errors.Is(%v, original) = false, want true", err)
	}
	for _, want := range []string{"review schema failed", "stage=review_2", "review-2.json"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}
}
