// Package app tests structured acceptance and QA evidence contracts.
package app

import "testing"

// TestValidateAcceptanceRequiresExecutableContracts keeps acceptance from becoming prose.
func TestValidateAcceptanceRequiresExecutableContracts(t *testing.T) {
	acceptance := Acceptance{
		Summary: "checkout acceptance",
		RequiredTests: []AcceptanceTest{{
			ID:      "contract-checkout",
			Source:  "change_contract",
			Path:    "docs/changes/demo/tests/checkout.acceptance.test.ts",
			Command: "pnpm exec tsx --test docs/changes/demo/tests/checkout.acceptance.test.ts",
			Purpose: "cover the checkout contract",
			Assertions: []string{
				"submitted checkout order remains visible after reload",
			},
		}},
		RequiredEvidence: []AcceptanceEvidence{{
			ID:      "screenshot-checkout",
			Kind:    "screenshot",
			Path:    "test-results/checkout/after-submit.png",
			Purpose: "prove the submitted order remains visible after reload",
		}},
	}
	if err := ValidateAcceptance(acceptance); err != nil {
		t.Fatal(err)
	}
	acceptance.RequiredTests = nil
	if err := ValidateAcceptance(acceptance); err == nil {
		t.Fatal("acceptance without required_tests should fail")
	}
}

// TestValidateQAAgainstAcceptanceRequiresEveryMatrixItem blocks partial evidence.
func TestValidateQAAgainstAcceptanceRequiresEveryMatrixItem(t *testing.T) {
	acceptance := Acceptance{
		Summary: "checkout acceptance",
		RequiredTests: []AcceptanceTest{{
			ID:      "contract-checkout",
			Source:  "change_contract",
			Path:    "docs/changes/demo/tests/checkout.acceptance.test.ts",
			Command: "pnpm exec tsx --test docs/changes/demo/tests/checkout.acceptance.test.ts",
			Purpose: "cover the checkout contract",
			Assertions: []string{
				"submitted checkout order remains visible after reload",
			},
		}},
		RequiredEvidence: []AcceptanceEvidence{{
			ID:      "screenshot-checkout",
			Kind:    "screenshot",
			Path:    "test-results/checkout/after-submit.png",
			Purpose: "prove the submitted order remains visible after reload",
		}},
	}
	qa := QA{
		Summary:  "qa ok",
		Decision: "clean",
		Evidence: []string{"Playwright screenshot artifact test-results/checkout/after-submit.png"},
		Findings: []Finding{},
		AcceptanceMatrix: []AcceptanceResult{{
			ID:       "contract-checkout",
			Status:   "passed",
			Artifact: "docs/changes/demo/tests/checkout.acceptance.test.ts",
			Evidence: "contract test passed",
		}},
	}
	if err := ValidateQAAgainstAcceptance(qa, acceptance); err == nil {
		t.Fatal("clean QA missing screenshot evidence matrix item should fail")
	}
	qa.AcceptanceMatrix = append(qa.AcceptanceMatrix, AcceptanceResult{
		ID:       "screenshot-checkout",
		Status:   "passed",
		Artifact: "test-results/checkout/after-submit.png",
		Evidence: "screenshot artifact shows submitted order after reload",
	})
	if err := ValidateQAAgainstAcceptance(qa, acceptance); err != nil {
		t.Fatal(err)
	}
}
