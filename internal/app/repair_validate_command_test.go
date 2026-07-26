// Package app tests the public repair artifact validation contract.
package app

import (
	"strings"
	"testing"
)

// TestValidateRepairRejectsUnverifiedClean verifies clean cannot bypass required checks.
func TestValidateRepairRejectsUnverifiedClean(t *testing.T) {
	repair := Review{
		Summary:  "未验证",
		Decision: "clean",
		Evidence: []string{"未执行命令"},
	}
	if err := ValidateRepair(repair); err == nil || !strings.Contains(err.Error(), "checks") {
		t.Fatalf("ValidateRepair error = %v, want false checks rejection", err)
	}
}

// TestValidateRepairRejectsBlankEvidence verifies whitespace is not accepted as proof.
func TestValidateRepairRejectsBlankEvidence(t *testing.T) {
	repair := cleanReviewForStageDecision()
	repair.Evidence = []string{" \t "}
	if err := ValidateRepair(repair); err == nil || !strings.Contains(err.Error(), "evidence") {
		t.Fatalf("ValidateRepair error = %v, want blank evidence rejection", err)
	}
}

// TestValidateRepairAcceptsVerifiedClean verifies validated command and runtime proof pass.
func TestValidateRepairAcceptsVerifiedClean(t *testing.T) {
	repair := cleanReviewForStageDecision()
	repair.Evidence = []string{"go test ./internal/app；runtime state verified"}
	if err := ValidateRepair(repair); err != nil {
		t.Fatalf("ValidateRepair rejected verified clean: %v", err)
	}
}
