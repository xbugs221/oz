// Package app blocks workflow execution when the sealed acceptance contract cannot be verified later.
package app

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/xbugs221/oz/internal/acceptance"
)

// AcceptancePreflightState records the deterministic contract check run after execution artifacts pass.
type AcceptancePreflightState struct {
	Attempts     int      `json:"attempts,omitempty"`
	Kind         string   `json:"kind,omitempty"`
	Status       string   `json:"status,omitempty"`
	LastArtifact string   `json:"last_artifact,omitempty"`
	LastError    string   `json:"last_error,omitempty"`
	Findings     []string `json:"findings,omitempty"`
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
		state.AcceptancePreflight.Status = validationStatusPassed
		state.AcceptancePreflight.Kind = validationKindAcceptancePreflight
		state.AcceptancePreflight.LastError = ""
		state.AcceptancePreflight.Findings = nil
		return true, nil
	}
	msg := strings.Join(findings, "; ")
	return false, recordAcceptancePreflightFailure(e.Repo, state, msg, findings)
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

// recordAcceptancePreflightFailure keeps producer trace errors inside same-stage repair.
func recordAcceptancePreflightFailure(repo string, state *State, message string, findings []string) error {
	// recordAcceptancePreflightFailure mirrors validation artifacts so retry prompts stay actionable.
	ensureWorkflowConfig(state)
	if state.Stages == nil {
		state.Stages = map[string]string{}
	}
	current := state.AcceptancePreflight
	current.Attempts++
	now := time.Now().UTC().Format(time.RFC3339Nano)
	attempt := ValidationAttempt{
		Stage:      state.Stage,
		Attempt:    current.Attempts,
		Status:     validationStatusFailed,
		StartedAt:  now,
		FinishedAt: now,
		Commands: []ValidationCommandResult{{
			Command:  acceptancePreflightGateCommand,
			ExitCode: 1,
			Output:   limitValidationOutput(preflightFailureOutput(message, findings)),
		}},
	}
	artifactPath, err := writeValidationAttempt(repo, state.RunID, attempt)
	if err != nil {
		return err
	}
	current.Kind = validationKindAcceptancePreflight
	current.Status = validationStatusFailed
	current.LastArtifact = artifactPath
	current.LastError = message
	current.Findings = append([]string(nil), findings...)
	state.AcceptancePreflight = current
	if current.Attempts >= state.Workflow.Validation.MaxAttemptsPerStage {
		state.Status = statusValidationBlocked
		state.Stage = statusValidationBlocked
		state.Error = current.LastError
		return nil
	}
	state.Stages[state.Stage] = "validation_failed"
	return nil
}

func preflightFailureOutput(message string, findings []string) string {
	// preflightFailureOutput gives agents both a compact message and exact finding list.
	payload := struct {
		Error    string   `json:"error"`
		Findings []string `json:"findings"`
	}{Error: message, Findings: findings}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Sprintf("%s\n%s", message, strings.Join(findings, "\n"))
	}
	return string(data)
}
