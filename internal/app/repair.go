// Package app validates durable self-review/self-repair checkpoints.
package app

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// ReadRepair loads one repair checkpoint while reusing the established finding schema.
func ReadRepair(path string) (Review, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Review{}, err
	}
	repair, err := parseReviewArtifact(path, data)
	if err != nil {
		return Review{}, err
	}
	repair = normalizeReview(repair)
	if err := ValidateRepair(repair); err != nil {
		return Review{}, ReviewArtifactError{Path: path, Code: reviewArtifactValidationError, Reason: err.Error()}
	}
	safeRepair, redacted := redactRepairEnvironmentMarkers(repair)
	if redacted {
		data, err := marshalRepairArtifact(safeRepair)
		if err != nil {
			return Review{}, err
		}
		mode := os.FileMode(0o644)
		if info, statErr := os.Stat(path); statErr == nil {
			mode = info.Mode().Perm()
		}
		if err := atomicWriteFile(path, append(data, '\n'), mode); err != nil {
			return Review{}, err
		}
		repair = safeRepair
	}
	return repair, nil
}

// ValidateRepair enforces the repair checkpoint decision and evidence contract.
func ValidateRepair(repair Review) error {
	if strings.TrimSpace(repair.Summary) == "" {
		return fmt.Errorf("repair summary 不能为空")
	}
	if repair.Decision != "clean" && repair.Decision != "needs_more" {
		return fmt.Errorf("无效 repair decision %q", repair.Decision)
	}
	if repair.Decision == "clean" && len(repair.Findings) != 0 {
		return fmt.Errorf("clean repair 不能包含 findings")
	}
	if repair.Decision == "clean" {
		if err := validateCleanReview(repair, 1); err != nil {
			return fmt.Errorf("clean repair 校验失败：%w", err)
		}
	}
	if repair.Decision == "needs_more" && len(repair.Findings) == 0 {
		return fmt.Errorf("needs_more repair 必须包含 findings")
	}
	if !hasNonBlankEvidence(repair.Evidence) {
		return fmt.Errorf("repair 必须包含验证 evidence")
	}
	for i, finding := range repair.Findings {
		if err := validateFinding(finding, fmt.Sprintf("finding %d", i), false); err != nil {
			return err
		}
	}
	for i, finding := range repair.NonBlockingFindings {
		if err := validateFinding(finding, fmt.Sprintf("non_blocking_findings %d", i), true); err != nil {
			return err
		}
	}
	return nil
}

// hasNonBlankEvidence rejects evidence arrays that contain only whitespace.
func hasNonBlankEvidence(evidence []string) bool {
	for _, item := range evidence {
		if strings.TrimSpace(item) != "" {
			return true
		}
	}
	return false
}

// RepairNeedsMore reports whether another durable repair round is required.
func RepairNeedsMore(repair Review) bool {
	return repair.Decision == "needs_more" || len(repair.Findings) > 0
}

// qualityEnvironmentNamesFromError extracts safe variable names or paths from an agent block marker.
func qualityEnvironmentNamesFromError(err error) []string {
	if err == nil {
		return nil
	}
	return qualityEnvironmentNamesFromText(err.Error())
}

// qualityEnvironmentNamesFromText extracts safe diagnostics from agent or gate output.
func qualityEnvironmentNamesFromText(text string) []string {
	const marker = "blocked_environment:"
	var names []string
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		index := strings.Index(line, marker)
		if index < 0 {
			continue
		}
		segments := strings.Split(line[index+len(marker):], ",")
		hasValuedItem := false
		for _, raw := range segments {
			_, _, hasValue := strings.Cut(raw, "=")
			hasValuedItem = hasValuedItem || hasValue
		}
		for _, raw := range segments {
			_, _, hasValue := strings.Cut(raw, "=")
			if hasValuedItem && !hasValue {
				continue
			}
			name := sanitizeEnvironmentName(raw)
			if name != "" && !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	return names
}

// redactQualityEnvironmentMarkers removes values from environment block diagnostics before persistence.
func redactQualityEnvironmentMarkers(text string) string {
	const marker = "blocked_environment:"
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		index := strings.Index(line, marker)
		if index < 0 {
			continue
		}
		names := qualityEnvironmentNamesFromText(line[index:])
		replacement := "[redacted]"
		if len(names) > 0 {
			replacement = strings.Join(names, ", ")
		}
		lines[i] = marker + " " + replacement
	}
	return strings.Join(lines, "\n")
}

// redactRepairEnvironmentMarkers removes marker values from every free-text repair field.
func redactRepairEnvironmentMarkers(repair Review) (Review, bool) {
	safe := repair
	changed := false
	redact := func(text string) string {
		result := redactQualityEnvironmentMarkers(text)
		if result != text {
			changed = true
		}
		return result
	}
	safe.Summary = redact(safe.Summary)
	safe.Evidence = append([]string(nil), safe.Evidence...)
	for i := range safe.Evidence {
		safe.Evidence[i] = redact(safe.Evidence[i])
	}
	safe.Findings = append([]Finding(nil), safe.Findings...)
	for i := range safe.Findings {
		safe.Findings[i].Title = redact(safe.Findings[i].Title)
		safe.Findings[i].Evidence = redact(safe.Findings[i].Evidence)
		safe.Findings[i].Recommendation = redact(safe.Findings[i].Recommendation)
	}
	safe.NonBlockingFindings = append([]Finding(nil), safe.NonBlockingFindings...)
	for i := range safe.NonBlockingFindings {
		safe.NonBlockingFindings[i].Title = redact(safe.NonBlockingFindings[i].Title)
		safe.NonBlockingFindings[i].Evidence = redact(safe.NonBlockingFindings[i].Evidence)
		safe.NonBlockingFindings[i].Recommendation = redact(safe.NonBlockingFindings[i].Recommendation)
	}
	if safe.WorkflowFailure != nil {
		failure := *safe.WorkflowFailure
		failure.Reason = redact(failure.Reason)
		safe.WorkflowFailure = &failure
	}
	return safe, changed
}

// marshalRepairArtifact preserves every strict repair field when writing a redacted checkpoint.
func marshalRepairArtifact(repair Review) ([]byte, error) {
	payload := struct {
		Summary             string                 `json:"summary"`
		Decision            string                 `json:"decision"`
		Evidence            []string               `json:"evidence"`
		Findings            []Finding              `json:"findings"`
		Checks              ReviewChecks           `json:"checks"`
		NonBlockingFindings []Finding              `json:"non_blocking_findings"`
		WorkflowFailure     *ReviewWorkflowFailure `json:"workflow_failure,omitempty"`
	}{
		Summary:             repair.Summary,
		Decision:            repair.Decision,
		Evidence:            repair.Evidence,
		Findings:            repair.Findings,
		Checks:              repair.Checks,
		NonBlockingFindings: repair.NonBlockingFindings,
		WorkflowFailure:     repair.WorkflowFailure,
	}
	if payload.Evidence == nil {
		payload.Evidence = []string{}
	}
	if payload.Findings == nil {
		payload.Findings = []Finding{}
	}
	if payload.NonBlockingFindings == nil {
		payload.NonBlockingFindings = []Finding{}
	}
	return json.MarshalIndent(payload, "", "  ")
}

// qualityEnvironmentNamesFromRepair lets a valid repair artifact request an environment pause.
func qualityEnvironmentNamesFromRepair(repair Review) []string {
	parts := append([]string{repair.Summary}, repair.Evidence...)
	for _, finding := range repair.Findings {
		parts = append(parts, finding.Title, finding.Evidence, finding.Recommendation)
	}
	return qualityEnvironmentNamesFromText(strings.Join(parts, "\n"))
}

// sanitizeEnvironmentName strips possible secret values and keeps only diagnostic identifiers.
func sanitizeEnvironmentName(raw string) string {
	name := strings.TrimSpace(raw)
	if before, _, found := strings.Cut(name, "="); found {
		name = strings.TrimSpace(before)
	}
	for _, r := range name {
		valid := r == '_' || r == '-' || r == '.' || r == '/' || r == '\\' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if !valid {
			return ""
		}
	}
	return name
}

// blockQualityEnvironment pauses a dynamic stage and binds recovery to dirty-path content at block time.
func blockQualityEnvironment(repo string, state *State, names []string) error {
	if state == nil {
		return nil
	}
	_, status, err := gitSnapshot(repo)
	if err != nil {
		return err
	}
	content, err := gitStatusContentSnapshot(repo, status)
	if err != nil {
		return err
	}
	if state.Stages != nil {
		state.Stages[state.Stage] = statusBlockedEnvironment
	}
	setQualityEnvironmentBlock(state, names, content)
	return nil
}

// setQualityEnvironmentBlock applies already-sanitized diagnostics and their content checkpoint.
func setQualityEnvironmentBlock(state *State, names []string, content string) {
	if state == nil {
		return
	}
	state.QualityLoop.BlockedFromStage = state.Stage
	state.QualityLoop.MissingEnvironmentNames = append([]string(nil), names...)
	state.QualityLoop.EnvironmentContent = content
	state.Status = statusBlockedEnvironment
	state.Stage = statusBlockedEnvironment
	state.Error = "缺少环境前置条件：" + strings.Join(names, ", ")
}
