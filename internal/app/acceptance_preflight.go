// Package app blocks workflow execution when the sealed acceptance contract cannot be verified later.
package app

import (
	"fmt"
	"os"
	"path/filepath"
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
	tests := map[string]AcceptanceTest{}
	for _, test := range contract.RequiredTests {
		tests[test.ID] = test
	}
	var findings []string
	for _, evidence := range contract.RequiredEvidence {
		if !acceptanceEvidenceHasProducer(repo, evidence, contract.Coverage, tests) {
			findings = append(findings, fmt.Sprintf("required_evidence %q 无法追溯到 required_tests producer", evidence.ID))
		}
	}
	return findings
}

// acceptanceEvidenceHasProducer requires a coverage link plus a concrete test or script producer.
func acceptanceEvidenceHasProducer(repo string, evidence AcceptanceEvidence, coverage []Coverage, tests map[string]AcceptanceTest) bool {
	for _, item := range coverage {
		if !stringSliceContains(item.Evidence, evidence.ID) {
			continue
		}
		for _, testID := range item.Tests {
			test, ok := tests[testID]
			if ok && (acceptanceTestMentionsEvidence(test, evidence) || acceptanceTestScriptProducesEvidence(repo, test, evidence)) {
				return true
			}
		}
	}
	return false
}

// acceptanceTestMentionsEvidence conservatively traces a runtime artifact to a test command or assertion.
func acceptanceTestMentionsEvidence(test AcceptanceTest, evidence AcceptanceEvidence) bool {
	needles := acceptanceEvidenceNeedles(evidence)
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

// acceptanceTestScriptProducesEvidence traces common shell wrappers that tee a test log beside the acceptance test.
func acceptanceTestScriptProducesEvidence(repo string, test AcceptanceTest, evidence AcceptanceEvidence) bool {
	if strings.TrimSpace(repo) == "" {
		return false
	}
	needles := acceptanceEvidenceNeedles(evidence)
	for _, relPath := range acceptanceProducerCandidatePaths(repo, test) {
		body, ok := readAcceptanceRelativeFile(repo, relPath)
		if !ok || !acceptanceTextMentionsAny(body, needles) {
			continue
		}
		if acceptanceProducerScriptMentionsTest(body, test) || relPath == test.Path {
			return true
		}
	}
	return false
}

// acceptanceProducerCandidatePaths returns declared test files and nearby shell wrappers worth inspecting.
func acceptanceProducerCandidatePaths(repo string, test AcceptanceTest) []string {
	seen := map[string]bool{}
	var paths []string
	add := func(path string) {
		path = strings.Trim(strings.TrimSpace(path), `"'`)
		if path == "" || strings.HasPrefix(path, "-") || filepath.IsAbs(path) {
			return
		}
		path = filepath.ToSlash(filepath.Clean(path))
		if path == "." || seen[path] {
			return
		}
		seen[path] = true
		paths = append(paths, path)
	}
	add(test.Path)
	for _, field := range strings.Fields(test.Command) {
		if strings.Contains(field, "/") {
			add(field)
		}
	}
	if test.Path == "" {
		return paths
	}
	dir := filepath.Dir(filepath.FromSlash(test.Path))
	entries, err := os.ReadDir(filepath.Join(repo, dir))
	if err != nil {
		return paths
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sh") {
			continue
		}
		add(filepath.ToSlash(filepath.Join(dir, entry.Name())))
	}
	return paths
}

// acceptanceProducerScriptMentionsTest keeps sibling wrappers tied to the declared required_test.
func acceptanceProducerScriptMentionsTest(body string, test AcceptanceTest) bool {
	if test.Path != "" && strings.Contains(body, test.Path) {
		return true
	}
	base := filepath.Base(filepath.FromSlash(test.Path))
	if base != "." && base != "" && strings.Contains(body, base) {
		return true
	}
	for _, field := range strings.Fields(test.Command) {
		field = strings.Trim(strings.TrimSpace(field), `"'`)
		if field != "" && strings.Contains(field, "/") && strings.Contains(body, field) {
			return true
		}
	}
	return false
}

// acceptanceEvidenceNeedles includes stable identifiers and log file names used by shell tee commands.
func acceptanceEvidenceNeedles(evidence AcceptanceEvidence) []string {
	var needles []string
	for _, needle := range []string{evidence.ID, evidence.Path} {
		needle = strings.TrimSpace(needle)
		if needle != "" {
			needles = append(needles, needle)
		}
	}
	if evidence.Path != "" {
		base := filepath.Base(filepath.FromSlash(evidence.Path))
		if base != "." && base != "" && base != evidence.Path {
			needles = append(needles, base)
		}
	}
	return needles
}

// acceptanceTextMentionsAny checks whether local test wrapper content names evidence output.
func acceptanceTextMentionsAny(body string, needles []string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(body, needle) {
			return true
		}
	}
	return false
}

// readAcceptanceRelativeFile reads only repo-relative producer candidates.
func readAcceptanceRelativeFile(repo string, relPath string) (string, bool) {
	relPath = filepath.Clean(filepath.FromSlash(relPath))
	if relPath == "." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) || filepath.IsAbs(relPath) {
		return "", false
	}
	body, err := os.ReadFile(filepath.Join(repo, relPath))
	if err != nil {
		return "", false
	}
	return string(body), true
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
