// Package acceptance provides a shared lifecycle diagnostics boundary for acceptance contracts.
package acceptance

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LifecycleDiagnostic records one machine-readable acceptance lifecycle finding.
type LifecycleDiagnostic struct {
	Code       string `json:"code"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	TestID     string `json:"test_id,omitempty"`
	EvidenceID string `json:"evidence_id,omitempty"`
	Path       string `json:"path,omitempty"`
}

// LifecycleResult records the shared validation, producer, and coverage view of a contract.
type LifecycleResult struct {
	Valid       bool                  `json:"valid"`
	Diagnostics []LifecycleDiagnostic `json:"diagnostics"`
	Required    RequiredItemSet       `json:"required"`
}

// RequiredItemSet exposes acceptance items that later QA stages must cover.
type RequiredItemSet struct {
	Tests    map[string]string `json:"tests"`
	Evidence map[string]string `json:"evidence"`
}

// ValidateLifecycle builds and returns shared lifecycle diagnostics for a contract.
func ValidateLifecycle(projectRoot string, contract Contract) LifecycleResult {
	// ValidateLifecycle keeps shape, path, producer, and coverage diagnostics in one boundary.
	result := BuildLifecycle(projectRoot, contract)
	result.Valid = len(result.Diagnostics) == 0
	return result
}

// BuildLifecycle collects lifecycle diagnostics without mutating the acceptance contract.
func BuildLifecycle(projectRoot string, contract Contract) LifecycleResult {
	// BuildLifecycle intentionally mirrors legacy checks while exposing structured diagnostics.
	required := RequiredItems(contract)
	result := LifecycleResult{Valid: true, Required: required}
	if err := Validate(contract); err != nil {
		result.Diagnostics = append(result.Diagnostics, LifecycleDiagnostic{
			Code:     "contract_shape",
			Severity: "error",
			Message:  "acceptance.json 无效：" + err.Error(),
		})
		return result
	}
	tests := map[string]Test{}
	for i, test := range contract.RequiredTests {
		tests[test.ID] = test
		if !validRelativePath(test.Path) {
			result.Diagnostics = append(result.Diagnostics, LifecycleDiagnostic{
				Code:     "required_test_path",
				Severity: "error",
				Message:  fmt.Sprintf("required_tests[%d].path 必须是相对测试路径：%s", i, test.Path),
				TestID:   test.ID,
				Path:     test.Path,
			})
			continue
		}
		if projectRoot != "" {
			testPath := filepath.Join(projectRoot, filepath.Clean(filepath.FromSlash(test.Path)))
			if info, err := os.Stat(testPath); err != nil || info.IsDir() {
				result.Diagnostics = append(result.Diagnostics, LifecycleDiagnostic{
					Code:     "required_test_missing",
					Severity: "error",
					Message:  fmt.Sprintf("required_tests[%d].path 指向的测试不存在：%s", i, test.Path),
					TestID:   test.ID,
					Path:     test.Path,
				})
			}
		}
		if !strings.Contains(test.Command, test.Path) {
			result.Diagnostics = append(result.Diagnostics, LifecycleDiagnostic{
				Code:     "required_test_command_path",
				Severity: "error",
				Message:  fmt.Sprintf("required_tests[%d].command 必须引用 path：%s", i, test.Path),
				TestID:   test.ID,
				Path:     test.Path,
			})
		}
	}
	for i, evidence := range contract.RequiredEvidence {
		if !validRelativePath(evidence.Path) {
			result.Diagnostics = append(result.Diagnostics, LifecycleDiagnostic{
				Code:       "required_evidence_path",
				Severity:   "error",
				Message:    fmt.Sprintf("required_evidence[%d].path 必须是相对产物路径：%s", i, evidence.Path),
				EvidenceID: evidence.ID,
				Path:       evidence.Path,
			})
		}
		if !EvidenceHasProducer(projectRoot, evidence, contract.Coverage, tests) {
			result.Diagnostics = append(result.Diagnostics, LifecycleDiagnostic{
				Code:       "required_evidence_producer_missing",
				Severity:   "error",
				Message:    fmt.Sprintf("required_evidence[%d] %q 无法追溯到 required_tests producer：必须在 coverage 绑定的 required_tests 的 command/purpose/assertions、测试文件或同目录 .sh wrapper 中明确产出 evidence id/path", i, evidence.ID),
				EvidenceID: evidence.ID,
				Path:       evidence.Path,
			})
		}
	}
	return result
}

// RequiredItems returns the required test and evidence ids grouped for QA coverage checks.
func RequiredItems(contract Contract) RequiredItemSet {
	// RequiredItems is the single source for acceptance_matrix required ids.
	required := RequiredItemSet{Tests: map[string]string{}, Evidence: map[string]string{}}
	for _, test := range contract.RequiredTests {
		required.Tests[test.ID] = test.Path
	}
	for _, evidence := range contract.RequiredEvidence {
		required.Evidence[evidence.ID] = evidence.Path
	}
	return required
}

func validRelativePath(path string) bool {
	// validRelativePath rejects empty, absolute, current-directory, and traversal paths.
	cleaned := filepath.Clean(filepath.FromSlash(strings.TrimSpace(path)))
	return cleaned != "" && cleaned != "." && !filepath.IsAbs(cleaned) && !strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) && cleaned != ".."
}
