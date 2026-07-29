// Package app validates structured QA artifacts produced by Codex.
package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/xbugs221/oz/internal/acceptance"
)

// QA is the strict JSON contract used by QA stages.
type QA struct {
	Summary             string             `json:"summary"`
	Decision            string             `json:"decision"`
	Evidence            []string           `json:"evidence"`
	Findings            []Finding          `json:"findings"`
	NonBlockingFindings []Finding          `json:"non_blocking_findings,omitempty"`
	AcceptanceMatrix    []AcceptanceResult `json:"acceptance_matrix,omitempty"`
	UserAcceptance      []UserAcceptance   `json:"user_acceptance,omitempty"`
}

// AcceptanceResult maps one acceptance contract item to QA proof.
type AcceptanceResult struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Artifact string `json:"artifact"`
	Evidence string `json:"evidence"`
}

// UserAcceptance records the user-visible result independently observed by final QA.
type UserAcceptance struct {
	ScenarioID  string   `json:"scenario_id"`
	Status      string   `json:"status"`
	Observed    string   `json:"observed"`
	EvidenceIDs []string `json:"evidence_ids"`
}

// UnmarshalJSON accepts compact numeric status codes while preserving reviewer observations.
func (r *UserAcceptance) UnmarshalJSON(data []byte) error {
	var raw struct {
		ScenarioID  string      `json:"scenario_id"`
		Status      interface{} `json:"status"`
		Observed    string      `json:"observed"`
		EvidenceIDs []string    `json:"evidence_ids"`
	}
	if err := decodeStrictArtifactJSON(data, &raw); err != nil {
		return err
	}
	r.ScenarioID = raw.ScenarioID
	r.Status = normalizeAcceptanceStatus(artifactScalarText(raw.Status))
	r.Observed = raw.Observed
	r.EvidenceIDs = raw.EvidenceIDs
	return nil
}

// UnmarshalJSON accepts KISS numeric status codes while storing canonical words.
func (r *AcceptanceResult) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID       string      `json:"id"`
		Status   interface{} `json:"status"`
		Artifact string      `json:"artifact"`
		Evidence string      `json:"evidence"`
	}
	if err := decodeStrictArtifactJSON(data, &raw); err != nil {
		return err
	}
	r.ID = raw.ID
	r.Status = normalizeAcceptanceStatus(artifactScalarText(raw.Status))
	r.Artifact = raw.Artifact
	r.Evidence = raw.Evidence
	return nil
}

// UnmarshalJSON accepts KISS numeric decision codes while storing canonical words.
func (qa *QA) UnmarshalJSON(data []byte) error {
	var raw struct {
		Summary             string             `json:"summary"`
		Decision            interface{}        `json:"decision"`
		Evidence            []string           `json:"evidence"`
		Findings            []Finding          `json:"findings"`
		NonBlockingFindings []Finding          `json:"non_blocking_findings,omitempty"`
		AcceptanceMatrix    []AcceptanceResult `json:"acceptance_matrix,omitempty"`
		UserAcceptance      []UserAcceptance   `json:"user_acceptance,omitempty"`
	}
	if err := decodeStrictArtifactJSON(data, &raw); err != nil {
		return err
	}
	qa.Summary = raw.Summary
	qa.Decision = normalizeDecision(artifactScalarText(raw.Decision))
	qa.Evidence = raw.Evidence
	qa.Findings = raw.Findings
	qa.NonBlockingFindings = raw.NonBlockingFindings
	qa.AcceptanceMatrix = raw.AcceptanceMatrix
	qa.UserAcceptance = raw.UserAcceptance
	return nil
}

// ReadQA loads and validates a QA JSON file.
func ReadQA(path string) (QA, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return QA{}, err
	}
	qa, err := parseQAArtifact(path, data)
	if err != nil {
		if artifactErr, ok := err.(ReviewArtifactError); ok {
			artifactErr.Path = path
			return QA{}, artifactErr
		}
		return QA{}, ReviewArtifactError{Path: path, Code: reviewArtifactParseError, Reason: err.Error()}
	}
	qa = normalizeQA(qa)
	if err := ValidateQA(qa); err != nil {
		return QA{}, ReviewArtifactError{Path: path, Code: reviewArtifactValidationError, Reason: err.Error()}
	}
	safeQA, redacted := redactQAEnvironmentMarkers(qa)
	if redacted {
		data, err := marshalQAArtifact(safeQA)
		if err != nil {
			return QA{}, err
		}
		mode := os.FileMode(0o644)
		if info, statErr := os.Stat(path); statErr == nil {
			mode = info.Mode().Perm()
		}
		if err := atomicWriteFile(path, append(data, '\n'), mode); err != nil {
			return QA{}, err
		}
		qa = safeQA
	}
	return qa, nil
}

// redactQAEnvironmentMarkers removes marker values from every free-text QA field.
func redactQAEnvironmentMarkers(qa QA) (QA, bool) {
	safe := qa
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
	safe.AcceptanceMatrix = append([]AcceptanceResult(nil), safe.AcceptanceMatrix...)
	for i := range safe.AcceptanceMatrix {
		safe.AcceptanceMatrix[i].Artifact = redact(safe.AcceptanceMatrix[i].Artifact)
		safe.AcceptanceMatrix[i].Evidence = redact(safe.AcceptanceMatrix[i].Evidence)
	}
	safe.UserAcceptance = append([]UserAcceptance(nil), safe.UserAcceptance...)
	for i := range safe.UserAcceptance {
		safe.UserAcceptance[i].Observed = redact(safe.UserAcceptance[i].Observed)
	}
	return safe, changed
}

// marshalQAArtifact preserves the strict QA shape when writing a redacted checkpoint.
func marshalQAArtifact(qa QA) ([]byte, error) {
	if qa.Evidence == nil {
		qa.Evidence = []string{}
	}
	if qa.Findings == nil {
		qa.Findings = []Finding{}
	}
	return json.MarshalIndent(qa, "", "  ")
}

// qualityEnvironmentNamesFromQA extracts safe environment identifiers from QA diagnostics.
func qualityEnvironmentNamesFromQA(qa QA) []string {
	parts := append([]string{qa.Summary}, qa.Evidence...)
	for _, finding := range qa.Findings {
		parts = append(parts, finding.Title, finding.Evidence, finding.Recommendation)
	}
	for _, finding := range qa.NonBlockingFindings {
		parts = append(parts, finding.Title, finding.Evidence, finding.Recommendation)
	}
	for _, result := range qa.AcceptanceMatrix {
		parts = append(parts, result.Artifact, result.Evidence)
	}
	for _, result := range qa.UserAcceptance {
		parts = append(parts, result.Observed)
	}
	return qualityEnvironmentNamesFromText(strings.Join(parts, "\n"))
}

// ValidateQA enforces the QA JSON schema used by the workflow.
func ValidateQA(qa QA) error {
	if strings.TrimSpace(qa.Summary) == "" {
		return fmt.Errorf("qa summary 不能为空")
	}
	if qa.Decision != "clean" && qa.Decision != "needs_fix" {
		return fmt.Errorf("无效 qa decision %q", qa.Decision)
	}
	if qa.Decision == "clean" {
		if len(qa.Findings) != 0 {
			return fmt.Errorf("clean qa 不能包含 findings")
		}
		if !hasRuntimeEvidence(qa.Evidence) {
			return fmt.Errorf("clean qa 必须包含可复核的运行时、截图、trace 或端到端测试 evidence")
		}
	}
	if qa.Decision == "needs_fix" && len(qa.Findings) == 0 {
		return fmt.Errorf("needs_fix qa 必须包含 findings")
	}
	for i, finding := range qa.Findings {
		if err := validateFinding(finding, fmt.Sprintf("finding %d", i), false); err != nil {
			return err
		}
	}
	for i, finding := range qa.NonBlockingFindings {
		if err := validateFinding(finding, fmt.Sprintf("non_blocking_findings %d", i), true); err != nil {
			return err
		}
	}
	for i, result := range qa.UserAcceptance {
		if strings.TrimSpace(result.ScenarioID) == "" || strings.TrimSpace(result.Observed) == "" || len(result.EvidenceIDs) == 0 {
			return fmt.Errorf("user_acceptance[%d] 不完整", i)
		}
		if result.Status != "passed" && result.Status != "failed" {
			return fmt.Errorf("user_acceptance[%d].status 无效：%s", i, result.Status)
		}
		if err := acceptance.ValidateUserFacingText(fmt.Sprintf("user_acceptance[%d].observed", i), result.Observed); err != nil {
			return err
		}
	}
	return nil
}

// ValidateQAAgainstAcceptance ensures every QA decision uses a complete, contract-owned matrix.
func ValidateQAAgainstAcceptance(qa QA, contract Acceptance) error {
	if err := ValidateQA(qa); err != nil {
		return err
	}
	if contract.DeliveryReport != nil {
		if err := validateQAUserAcceptance(qa, *contract.DeliveryReport); err != nil {
			return err
		}
	}
	required := acceptance.RequiredItems(contract)
	if len(required.Tests) == 0 && len(required.Evidence) == 0 {
		return nil
	}
	seen := map[string]bool{}
	for i, result := range qa.AcceptanceMatrix {
		if strings.TrimSpace(result.ID) == "" || strings.TrimSpace(result.Status) == "" || strings.TrimSpace(result.Evidence) == "" {
			return fmt.Errorf("acceptance_matrix[%d] 不完整", i)
		}
		_, testOK := required.Tests[result.ID]
		_, evidenceOK := required.Evidence[result.ID]
		if !testOK && !evidenceOK {
			return fmt.Errorf("acceptance_matrix[%d].id 未在 acceptance 合同中定义：%q", i, result.ID)
		}
		if result.Status != "passed" && result.Status != "failed" {
			return fmt.Errorf("acceptance_matrix[%d].status 无效：%s", i, result.Status)
		}
		if qa.Decision == "clean" && result.Status != "passed" {
			return fmt.Errorf("acceptance_matrix[%d] 未通过：%s", i, result.ID)
		}
		seen[result.ID] = true
	}
	for id := range required.Tests {
		if !seen[id] {
			return fmt.Errorf("clean qa 缺少 required_tests acceptance_matrix 覆盖：%s", id)
		}
	}
	for id := range required.Evidence {
		if !seen[id] {
			return fmt.Errorf("clean qa 缺少 required_evidence acceptance_matrix 覆盖：%s", id)
		}
	}
	return nil
}

// validateQAUserAcceptance requires clean QA to cover every delivery scenario with observed, linked evidence.
func validateQAUserAcceptance(qa QA, report acceptance.DeliveryReport) error {
	if qa.Decision != "clean" {
		return nil
	}
	scenarios := make(map[string]acceptance.DeliveryScenario, len(report.Scenarios))
	for _, scenario := range report.Scenarios {
		scenarios[scenario.ID] = scenario
	}
	seen := map[string]bool{}
	for i, result := range qa.UserAcceptance {
		scenario, ok := scenarios[result.ScenarioID]
		if !ok {
			return fmt.Errorf("user_acceptance[%d].scenario_id 未在 delivery_report 中定义：%q", i, result.ScenarioID)
		}
		if seen[result.ScenarioID] {
			return fmt.Errorf("user_acceptance[%d].scenario_id 重复：%q", i, result.ScenarioID)
		}
		seen[result.ScenarioID] = true
		if result.Status != "passed" {
			return fmt.Errorf("clean qa 的用户验收场景未通过：%s", result.ScenarioID)
		}
		allowed := map[string]bool{}
		for _, id := range scenario.EvidenceIDs {
			allowed[id] = true
		}
		referenced := map[string]bool{}
		for _, id := range result.EvidenceIDs {
			if !allowed[id] {
				return fmt.Errorf("user_acceptance[%d].evidence_ids 引用场景外证据：%q", i, id)
			}
			if referenced[id] {
				return fmt.Errorf("user_acceptance[%d].evidence_ids 重复：%q", i, id)
			}
			referenced[id] = true
		}
		for id := range allowed {
			if !referenced[id] {
				return fmt.Errorf("user_acceptance[%d] 缺少场景证据：%s", i, id)
			}
		}
	}
	for id := range scenarios {
		if !seen[id] {
			return fmt.Errorf("clean qa 缺少用户验收场景：%s", id)
		}
	}
	return nil
}

func normalizeQA(qa QA) QA {
	qa.Decision = normalizeDecision(qa.Decision)
	for i := range qa.Findings {
		if severity, ok := normalizeFindingSeverity(qa.Findings[i].Severity); ok {
			qa.Findings[i].Severity = severity
		}
		if scope, ok := normalizeFindingScope(qa.Findings[i].Scope); ok {
			qa.Findings[i].Scope = scope
		}
	}
	for i := range qa.NonBlockingFindings {
		if severity, ok := normalizeFindingSeverity(qa.NonBlockingFindings[i].Severity); ok {
			qa.NonBlockingFindings[i].Severity = severity
		}
		if scope, ok := normalizeFindingScope(qa.NonBlockingFindings[i].Scope); ok {
			qa.NonBlockingFindings[i].Scope = scope
		}
	}
	for i := range qa.AcceptanceMatrix {
		qa.AcceptanceMatrix[i].Status = normalizeAcceptanceStatus(qa.AcceptanceMatrix[i].Status)
	}
	for i := range qa.UserAcceptance {
		qa.UserAcceptance[i].Status = normalizeAcceptanceStatus(qa.UserAcceptance[i].Status)
	}
	return qa
}

func parseQAArtifact(path string, data []byte) (QA, error) {
	var qa QA
	cleaned := bytes.TrimSpace(data)
	cleaned = bytes.TrimPrefix(cleaned, []byte{0xef, 0xbb, 0xbf})
	if len(cleaned) == 0 {
		return QA{}, ReviewArtifactError{Path: path, Code: reviewArtifactParseError, Reason: "artifact is empty"}
	}
	dec := json.NewDecoder(bytes.NewReader(cleaned))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&qa); err != nil {
		return QA{}, ReviewArtifactError{Path: path, Code: reviewArtifactParseError, Reason: err.Error()}
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return QA{}, ReviewArtifactError{Path: path, Code: reviewArtifactParseError, Reason: "artifact contains trailing content; output must be a single JSON object"}
	}
	return qa, nil
}

// QANeedsFix reports whether a valid QA artifact requires another fix round.
func QANeedsFix(qa QA) bool {
	return qa.Decision == "needs_fix" || len(qa.Findings) > 0
}

// qaArtifactContentHash binds targeted repair to every normalized source QA field.
func qaArtifactContentHash(qa QA) string {
	data, err := json.Marshal(normalizeQA(qa))
	if err != nil {
		return ""
	}
	return qualityHashStrings(string(data))
}
