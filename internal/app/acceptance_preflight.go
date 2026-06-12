// Package app blocks workflow execution when the sealed acceptance contract cannot be verified later.
package app

import (
	"fmt"
	"strings"
)

// AcceptancePreflightState records the deterministic contract check run after execution artifacts pass.
type AcceptancePreflightState struct {
	Status    string   `json:"status,omitempty"`
	LastError string   `json:"last_error,omitempty"`
	Findings  []string `json:"findings,omitempty"`
}

// runAcceptancePreflight checks that required evidence can be traced to a required test producer.
func (e *Engine) runAcceptancePreflight(state *State) (bool, error) {
	if state.Stage != "execution" || state.Status != statusRunning {
		return true, nil
	}
	contract, err := readAcceptanceForState(e.Repo, *state)
	if err != nil {
		return false, err
	}
	findings := acceptancePreflightFindings(contract)
	if len(findings) == 0 {
		state.AcceptancePreflight = AcceptancePreflightState{Status: validationStatusPassed}
		return true, nil
	}
	msg := strings.Join(findings, "; ")
	state.AcceptancePreflight = AcceptancePreflightState{
		Status:    validationStatusFailed,
		LastError: msg,
		Findings:  findings,
	}
	state.Status = statusAcceptanceContractBlocked
	state.Stage = statusAcceptanceContractBlocked
	state.Error = msg
	return false, nil
}

// acceptancePreflightFindings returns contract issues that would waste later review/QA/fix turns.
func acceptancePreflightFindings(contract Acceptance) []string {
	tests := map[string]AcceptanceTest{}
	for _, test := range contract.RequiredTests {
		tests[test.ID] = test
	}
	var findings []string
	for _, evidence := range contract.RequiredEvidence {
		if !acceptanceEvidenceHasProducer(evidence, contract.Coverage, tests) {
			findings = append(findings, fmt.Sprintf("required_evidence %q 无法追溯到 required_tests producer", evidence.ID))
		}
	}
	return findings
}

// acceptanceEvidenceHasProducer requires a coverage link plus a concrete test text mention.
func acceptanceEvidenceHasProducer(evidence AcceptanceEvidence, coverage []Coverage, tests map[string]AcceptanceTest) bool {
	for _, item := range coverage {
		if !stringSliceContains(item.Evidence, evidence.ID) {
			continue
		}
		for _, testID := range item.Tests {
			test, ok := tests[testID]
			if ok && acceptanceTestMentionsEvidence(test, evidence) {
				return true
			}
		}
	}
	return false
}

// acceptanceTestMentionsEvidence conservatively traces a runtime artifact to a test command or assertion.
func acceptanceTestMentionsEvidence(test AcceptanceTest, evidence AcceptanceEvidence) bool {
	needles := []string{strings.TrimSpace(evidence.ID), strings.TrimSpace(evidence.Path)}
	haystacks := []string{test.ID, test.Path, test.Command, test.Purpose}
	haystacks = append(haystacks, test.Assertions...)
	for _, needle := range needles {
		if needle == "" {
			continue
		}
		for _, haystack := range haystacks {
			if strings.Contains(haystack, needle) {
				return true
			}
		}
	}
	return false
}

func stringSliceContains(values []string, want string) bool {
	// stringSliceContains keeps preflight matching explicit and case-sensitive for contract ids.
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
