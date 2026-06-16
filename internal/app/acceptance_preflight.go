// Package app blocks workflow execution when the sealed acceptance contract cannot be verified later.
package app

import (
	"strings"

	"github.com/xbugs221/oz/internal/acceptance"
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
	findings := acceptancePreflightFindings(e.Repo, contract)
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
func acceptancePreflightFindings(repo string, contract Acceptance) []string {
	var findings []string
	lifecycle := acceptance.ValidateLifecycle(repo, contract)
	for _, diagnostic := range lifecycle.Diagnostics {
		if diagnostic.Code == "required_evidence_producer_missing" {
			findings = append(findings, diagnostic.Message)
		}
	}
	return findings
}
