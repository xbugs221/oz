// Package acceptance validates structured oz and wo acceptance contracts.
package acceptance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var weakAssertionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^\s*http\s*200\s*$`),
	regexp.MustCompile(`(?i)^\s*(status\s*)?(code\s*)?200\s*$`),
	regexp.MustCompile(`(?i)^\s*2xx\s*$`),
	regexp.MustCompile(`^\s*元素存在\s*$`),
	regexp.MustCompile(`^\s*组件渲染成功\s*$`),
	regexp.MustCompile(`^\s*页面能打开\s*$`),
}

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
		if len(test.Assertions) == 0 {
			return fmt.Errorf("required_tests[%d].assertions 至少包含一个业务级断言", i)
		}
		for j, assertion := range test.Assertions {
			if strings.TrimSpace(assertion) == "" {
				return fmt.Errorf("required_tests[%d].assertions[%d] 不能为空", i, j)
			}
			if weakAssertion(assertion) {
				return fmt.Errorf("required_tests[%d].assertions[%d] 是弱验收断言：%q", i, j, assertion)
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

// EvidenceHasProducer reports whether an evidence artifact is tied to a required test producer.
func EvidenceHasProducer(projectRoot string, evidence Evidence, coverage []Coverage, tests map[string]Test) bool {
	for _, item := range coverage {
		if !stringSliceContains(item.Evidence, evidence.ID) {
			continue
		}
		for _, testID := range item.Tests {
			test, ok := tests[testID]
			if ok && (testMentionsEvidence(test, evidence) || testScriptProducesEvidence(projectRoot, test, evidence)) {
				return true
			}
		}
	}
	return false
}

func testMentionsEvidence(test Test, evidence Evidence) bool {
	// testMentionsEvidence conservatively traces a runtime artifact to required_test metadata.
	needles := evidenceNeedles(evidence)
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

func testScriptProducesEvidence(projectRoot string, test Test, evidence Evidence) bool {
	// testScriptProducesEvidence inspects the declared test and nearby shell wrappers.
	if strings.TrimSpace(projectRoot) == "" {
		return false
	}
	needles := evidenceNeedles(evidence)
	for _, relPath := range producerCandidatePaths(projectRoot, test) {
		body, ok := readRelativeFile(projectRoot, relPath)
		if !ok || !textMentionsAny(body, needles) {
			continue
		}
		if producerScriptMentionsTest(body, test) || relPath == test.Path {
			return true
		}
	}
	return false
}

func producerCandidatePaths(projectRoot string, test Test) []string {
	// producerCandidatePaths returns declared test files and sibling shell wrappers.
	seen := map[string]bool{}
	paths := []string{}
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
	entries, err := os.ReadDir(filepath.Join(projectRoot, dir))
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

func producerScriptMentionsTest(body string, test Test) bool {
	// producerScriptMentionsTest keeps sibling wrappers tied to the declared required_test.
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

func evidenceNeedles(evidence Evidence) []string {
	// evidenceNeedles includes stable identifiers and artifact names.
	needles := []string{}
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

func textMentionsAny(body string, needles []string) bool {
	// textMentionsAny checks whether local producer content names evidence output.
	for _, needle := range needles {
		if needle != "" && strings.Contains(body, needle) {
			return true
		}
	}
	return false
}

func readRelativeFile(projectRoot, relPath string) (string, bool) {
	// readRelativeFile reads only paths under the validated project.
	relPath = filepath.Clean(filepath.FromSlash(relPath))
	if relPath == "." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) || filepath.IsAbs(relPath) {
		return "", false
	}
	body, err := os.ReadFile(filepath.Join(projectRoot, relPath))
	if err != nil {
		return "", false
	}
	return string(body), true
}

func stringSliceContains(values []string, want string) bool {
	// stringSliceContains keeps contract id matching exact and case-sensitive.
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func weakAssertion(assertion string) bool {
	// weakAssertion rejects clear surface checks that do not describe business behavior.
	trimmed := strings.TrimSpace(assertion)
	for _, pattern := range weakAssertionPatterns {
		if pattern.MatchString(trimmed) {
			return true
		}
	}
	return false
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
