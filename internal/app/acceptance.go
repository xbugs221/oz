// Package app validates structured acceptance contracts used by workflow QA.
package app

import (
	"os"

	"github.com/xbugs221/oz/internal/acceptance"
)

// Acceptance is the JSON contract produced before implementation starts.
type Acceptance = acceptance.Contract

// AcceptanceTest records one executable test command that later stages must pass.
type AcceptanceTest = acceptance.Test

// AcceptanceEvidence records one runtime artifact that QA must collect.
type AcceptanceEvidence = acceptance.Evidence

// Coverage links spec scenarios to concrete tests and QA evidence.
type Coverage = acceptance.Coverage

// ReadAcceptance loads and validates the acceptance JSON file.
func ReadAcceptance(path string) (Acceptance, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Acceptance{}, err
	}
	contract, err := acceptance.Parse(data)
	if err != nil {
		return Acceptance{}, ReviewArtifactError{Path: path, Code: reviewArtifactParseError, Reason: err.Error()}
	}
	return contract, nil
}

// ValidateAcceptance enforces the pre-implementation acceptance contract shape.
func ValidateAcceptance(contract Acceptance) error {
	return acceptance.Validate(contract)
}
