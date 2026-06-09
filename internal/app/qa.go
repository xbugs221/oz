// Package app validates structured QA artifacts produced by Codex.
package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// QA is the strict JSON contract used by QA stages.
type QA struct {
	Summary          string             `json:"summary"`
	Decision         string             `json:"decision"`
	Evidence         []string           `json:"evidence"`
	Findings         []Finding          `json:"findings"`
	AcceptanceMatrix []AcceptanceResult `json:"acceptance_matrix,omitempty"`
}

// AcceptanceResult maps one acceptance contract item to QA proof.
type AcceptanceResult struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Artifact string `json:"artifact"`
	Evidence string `json:"evidence"`
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
	return qa, nil
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
		if finding.Title == "" || finding.Evidence == "" || finding.Recommendation == "" {
			return fmt.Errorf("finding %d 不完整", i)
		}
		if _, ok := normalizeFindingSeverity(finding.Severity); !ok {
			return fmt.Errorf("finding %d 的 severity 无效：%q", i, finding.Severity)
		}
	}
	return nil
}

// ValidateQAAgainstAcceptance ensures clean QA covers every acceptance item.
func ValidateQAAgainstAcceptance(qa QA, acceptance Acceptance) error {
	if err := ValidateQA(qa); err != nil {
		return err
	}
	if qa.Decision != "clean" {
		return nil
	}
	required := map[string]string{}
	for _, test := range acceptance.RequiredTests {
		required[test.ID] = "required_tests"
	}
	for _, evidence := range acceptance.RequiredEvidence {
		required[evidence.ID] = "required_evidence"
	}
	if len(required) == 0 {
		return nil
	}
	seen := map[string]bool{}
	for i, result := range qa.AcceptanceMatrix {
		if strings.TrimSpace(result.ID) == "" || strings.TrimSpace(result.Status) == "" || strings.TrimSpace(result.Evidence) == "" {
			return fmt.Errorf("acceptance_matrix[%d] 不完整", i)
		}
		if _, ok := required[result.ID]; !ok {
			return fmt.Errorf("acceptance_matrix[%d].id 未在 acceptance 合同中定义：%q", i, result.ID)
		}
		if result.Status != "passed" {
			return fmt.Errorf("acceptance_matrix[%d] 未通过：%s", i, result.ID)
		}
		seen[result.ID] = true
	}
	for id, group := range required {
		if !seen[id] {
			return fmt.Errorf("clean qa 缺少 %s acceptance_matrix 覆盖：%s", group, id)
		}
	}
	return nil
}

func normalizeQA(qa QA) QA {
	for i := range qa.Findings {
		if severity, ok := normalizeFindingSeverity(qa.Findings[i].Severity); ok {
			qa.Findings[i].Severity = severity
		}
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
