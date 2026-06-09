// Package acceptance validates structured oz and wo acceptance contracts.
package acceptance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Contract is the JSON contract produced before implementation starts.
type Contract struct {
	Summary          string     `json:"summary"`
	Coverage         []Coverage `json:"coverage,omitempty"`
	RequiredTests    []Test     `json:"required_tests"`
	RequiredEvidence []Evidence `json:"required_evidence"`
}

// Coverage links spec scenarios to concrete tests and QA evidence.
type Coverage struct {
	Spec     string   `json:"spec"`
	Tests    []string `json:"tests"`
	Evidence []string `json:"evidence"`
	Risk     string   `json:"risk"`
}

// Test records one executable test command that later stages must pass.
type Test struct {
	ID                     string   `json:"id"`
	Source                 string   `json:"source"`
	Path                   string   `json:"path"`
	Command                string   `json:"command"`
	Purpose                string   `json:"purpose"`
	Assertions             []string `json:"assertions,omitempty"`
	ExpectedInitialFailure string   `json:"expected_initial_failure,omitempty"`
}

// Evidence records one runtime artifact that QA must collect.
type Evidence struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	Purpose string `json:"purpose"`
}

// Read loads and validates the acceptance JSON file.
func Read(path string) (Contract, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Contract{}, err
	}
	return Parse(data)
}

// Parse strictly decodes and validates an acceptance JSON document.
func Parse(data []byte) (Contract, error) {
	var contract Contract
	cleaned := bytes.TrimSpace(data)
	cleaned = bytes.TrimPrefix(cleaned, []byte{0xef, 0xbb, 0xbf})
	if len(cleaned) == 0 {
		return Contract{}, fmt.Errorf("artifact is empty")
	}
	dec := json.NewDecoder(bytes.NewReader(cleaned))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&contract); err != nil {
		return Contract{}, err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return Contract{}, fmt.Errorf("artifact contains trailing content; output must be a single JSON object")
	}
	if err := Validate(contract); err != nil {
		return Contract{}, err
	}
	return contract, nil
}

// Validate enforces the pre-implementation acceptance contract shape.
func Validate(contract Contract) error {
	if strings.TrimSpace(contract.Summary) == "" {
		return fmt.Errorf("acceptance summary 不能为空")
	}
	if len(contract.RequiredTests) == 0 {
		return fmt.Errorf("acceptance required_tests 至少包含一个测试")
	}
	testIDs := map[string]bool{}
	for i, test := range contract.RequiredTests {
		if strings.TrimSpace(test.ID) == "" || strings.TrimSpace(test.Path) == "" || strings.TrimSpace(test.Command) == "" || strings.TrimSpace(test.Purpose) == "" {
			return fmt.Errorf("required_tests[%d] 不完整", i)
		}
		if !validTestSource(test.Source) {
			return fmt.Errorf("required_tests[%d].source 无效：%q", i, test.Source)
		}
		if testIDs[test.ID] {
			return fmt.Errorf("required_tests[%d].id 重复：%q", i, test.ID)
		}
		for j, assertion := range test.Assertions {
			if strings.TrimSpace(assertion) == "" {
				return fmt.Errorf("required_tests[%d].assertions[%d] 不能为空", i, j)
			}
		}
		testIDs[test.ID] = true
	}
	evidenceIDs := map[string]bool{}
	for i, evidence := range contract.RequiredEvidence {
		if strings.TrimSpace(evidence.ID) == "" || strings.TrimSpace(evidence.Kind) == "" || strings.TrimSpace(evidence.Path) == "" || strings.TrimSpace(evidence.Purpose) == "" {
			return fmt.Errorf("required_evidence[%d] 不完整", i)
		}
		if !validEvidenceKind(evidence.Kind) {
			return fmt.Errorf("required_evidence[%d].kind 无效：%q", i, evidence.Kind)
		}
		if evidenceIDs[evidence.ID] {
			return fmt.Errorf("required_evidence[%d].id 重复：%q", i, evidence.ID)
		}
		evidenceIDs[evidence.ID] = true
	}
	for i, coverage := range contract.Coverage {
		if strings.TrimSpace(coverage.Spec) == "" {
			return fmt.Errorf("coverage[%d].spec 不能为空", i)
		}
		if len(coverage.Tests) == 0 {
			return fmt.Errorf("coverage[%d].tests 至少引用一个 required_tests id", i)
		}
		for j, id := range coverage.Tests {
			if strings.TrimSpace(id) == "" {
				return fmt.Errorf("coverage[%d].tests[%d] 不能为空", i, j)
			}
			if !testIDs[id] {
				return fmt.Errorf("coverage[%d].tests[%d] 引用未知 required_tests id：%q", i, j, id)
			}
		}
		for j, id := range coverage.Evidence {
			if strings.TrimSpace(id) == "" {
				return fmt.Errorf("coverage[%d].evidence[%d] 不能为空", i, j)
			}
			if !evidenceIDs[id] {
				return fmt.Errorf("coverage[%d].evidence[%d] 引用未知 required_evidence id：%q", i, j, id)
			}
		}
		if len(coverage.Evidence) == 0 && strings.TrimSpace(coverage.Risk) == "" {
			return fmt.Errorf("coverage[%d].risk 必须说明无证据覆盖的剩余风险", i)
		}
	}
	return nil
}

func validTestSource(source string) bool {
	// validTestSource matches the existing wo sealed-run schema.
	switch source {
	case "change_contract", "root_e2e", "existing_regression", "new_regression":
		return true
	default:
		return false
	}
}

func validEvidenceKind(kind string) bool {
	// validEvidenceKind matches the existing wo sealed-run schema.
	switch kind {
	case "screenshot", "trace", "network", "console", "runtime_log", "state_snapshot", "other":
		return true
	default:
		return false
	}
}
