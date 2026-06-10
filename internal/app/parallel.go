// Package app defines the optional parallel helper artifact contract for sealed runs.
package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ParallelArtifact stores the auditable result of one configured helper group.
type ParallelArtifact struct {
	Group   string                 `json:"group"`
	Mode    string                 `json:"mode"`
	Members []ParallelMemberResult `json:"members"`
	Summary string                 `json:"summary"`
}

// ParallelMemberResult stores one helper member's summary and evidence.
type ParallelMemberResult struct {
	Name     string    `json:"name"`
	Purpose  string    `json:"purpose"`
	Status   string    `json:"status"`
	Summary  string    `json:"summary"`
	Evidence []string  `json:"evidence,omitempty"`
	Findings []Finding `json:"findings,omitempty"`
	Required bool      `json:"required,omitempty"`
}

// UnmarshalJSON accepts KISS numeric status codes while storing canonical words.
func (m *ParallelMemberResult) UnmarshalJSON(data []byte) error {
	var raw struct {
		Name     string      `json:"name"`
		Purpose  string      `json:"purpose"`
		Status   interface{} `json:"status"`
		Summary  string      `json:"summary"`
		Evidence []string    `json:"evidence,omitempty"`
		Findings []Finding   `json:"findings,omitempty"`
		Required bool        `json:"required,omitempty"`
	}
	if err := decodeStrictArtifactJSON(data, &raw); err != nil {
		return err
	}
	m.Name = raw.Name
	m.Purpose = raw.Purpose
	m.Status = normalizeMemberStatus(artifactScalarText(raw.Status))
	m.Summary = raw.Summary
	m.Evidence = raw.Evidence
	m.Findings = raw.Findings
	m.Required = raw.Required
	return nil
}

// ReadParallelArtifact loads the run-local helper result for an enabled group.
func ReadParallelArtifact(path string) (ParallelArtifact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ParallelArtifact{}, err
	}
	artifact, err := parseParallelArtifact(path, data)
	if err != nil {
		if artifactErr, ok := err.(ReviewArtifactError); ok {
			artifactErr.Path = path
			return ParallelArtifact{}, artifactErr
		}
		return ParallelArtifact{}, ReviewArtifactError{Path: path, Code: reviewArtifactParseError, Reason: err.Error()}
	}
	if err := ValidateParallelArtifact(artifact); err != nil {
		return ParallelArtifact{}, ReviewArtifactError{Path: path, Code: reviewArtifactValidationError, Reason: err.Error()}
	}
	return artifact, nil
}

// ValidateParallelArtifact enforces the auditable helper artifact boundary.
func ValidateParallelArtifact(artifact ParallelArtifact) error {
	if strings.TrimSpace(artifact.Group) == "" {
		return fmt.Errorf("parallel artifact group 不能为空")
	}
	if strings.TrimSpace(artifact.Mode) == "" {
		return fmt.Errorf("parallel artifact mode 不能为空")
	}
	if strings.TrimSpace(artifact.Summary) == "" {
		return fmt.Errorf("parallel artifact summary 不能为空")
	}
	if len(artifact.Members) == 0 {
		return fmt.Errorf("parallel artifact 必须包含 members")
	}
	for i, member := range artifact.Members {
		if strings.TrimSpace(member.Name) == "" || strings.TrimSpace(member.Status) == "" || strings.TrimSpace(member.Summary) == "" {
			return fmt.Errorf("parallel artifact member %d 不完整", i)
		}
		for j, finding := range member.Findings {
			if finding.Title == "" || finding.Evidence == "" || finding.Recommendation == "" {
				return fmt.Errorf("parallel artifact member %d finding %d 不完整", i, j)
			}
			if _, ok := normalizeFindingSeverity(finding.Severity); !ok {
				return fmt.Errorf("parallel artifact member %d finding %d 的 severity 无效：%q", i, j, finding.Severity)
			}
			if _, ok := normalizeFindingScope(finding.Scope); !ok {
				return fmt.Errorf("parallel artifact member %d finding %d 的 scope 无效：%q", i, j, finding.Scope)
			}
		}
	}
	return nil
}

// ValidateParallelReviewGate blocks clean review when enabled helper findings were ignored.
func ValidateParallelReviewGate(runPath string, workflow WorkflowConfig, iteration int, review Review) error {
	artifact, ok, err := readEnabledParallelArtifact(runPath, workflow, "review", iteration)
	if err != nil || !ok {
		return err
	}
	if review.Decision == "clean" && (artifactHasSevereFinding(artifact) || artifactHasMemberFailure(artifact)) {
		return fmt.Errorf("clean review 不得忽略 parallel-review-%d.json 中的 blocker/major gate_input finding 或成员失败", iteration)
	}
	return nil
}

// ValidateParallelQAGate blocks clean QA when enabled helper failures were ignored.
func ValidateParallelQAGate(runPath string, workflow WorkflowConfig, iteration int, qa QA) error {
	artifact, ok, err := readEnabledParallelArtifact(runPath, workflow, "qa", iteration)
	if err != nil || !ok {
		return err
	}
	if qa.Decision == "clean" && (artifactHasSevereFinding(artifact) || artifactHasMemberFailure(artifact)) {
		return fmt.Errorf("clean qa 不得忽略 parallel-qa-%d.json 中的 gate_input finding 或成员失败", iteration)
	}
	return nil
}

// ValidateParallelContextGate blocks execution when required implementation context failed.
func ValidateParallelContextGate(runPath string, workflow WorkflowConfig) error {
	for _, group := range []string{"planning_context", "implementation_context"} {
		artifact, ok, err := readEnabledParallelArtifact(runPath, workflow, group, 0)
		if err != nil || !ok {
			return err
		}
		if artifactHasRequiredFailure(artifact) {
			return fmt.Errorf("%s 中 required 成员失败", filepath.Base(parallelArtifactPath(runPath, group, 0)))
		}
	}
	return nil
}

func readEnabledParallelArtifact(runPath string, workflow WorkflowConfig, group string, iteration int) (ParallelArtifact, bool, error) {
	if !workflow.Parallel.Enabled {
		return ParallelArtifact{}, false, nil
	}
	config, ok := workflow.Parallel.Groups[group]
	if !ok {
		return ParallelArtifact{}, false, nil
	}
	path := parallelArtifactPath(runPath, group, iteration)
	artifact, err := ReadParallelArtifact(path)
	if err != nil {
		return ParallelArtifact{}, true, err
	}
	if err := ValidateParallelArtifactForGroup(artifact, group, config); err != nil {
		return ParallelArtifact{}, true, err
	}
	return artifact, true, nil
}

// ValidateParallelArtifactForGroup proves helper output exactly matches the configured group.
func ValidateParallelArtifactForGroup(artifact ParallelArtifact, group string, config ParallelGroupConfig) error {
	if artifact.Group != group {
		return fmt.Errorf("parallel artifact group %q 不匹配配置 group %q", artifact.Group, group)
	}
	if artifact.Mode != config.Mode {
		return fmt.Errorf("parallel artifact mode %q 不匹配配置 mode %q", artifact.Mode, config.Mode)
	}
	configured := map[string]bool{}
	configuredNames := []string{}
	for _, member := range config.Members {
		name := strings.TrimSpace(member.Name)
		if name == "" {
			continue
		}
		if configured[name] {
			continue
		}
		configured[name] = true
		configuredNames = append(configuredNames, name)
	}
	seen := map[string]bool{}
	unconfigured := []string{}
	for _, member := range artifact.Members {
		name := strings.TrimSpace(member.Name)
		if seen[name] {
			return fmt.Errorf("parallel artifact member 重复：%s", name)
		}
		if !configured[name] {
			unconfigured = append(unconfigured, name)
			seen[name] = true
			continue
		}
		seen[name] = true
	}
	if len(unconfigured) > 0 {
		sort.Strings(unconfigured)
		return fmt.Errorf("parallel artifact 包含未配置成员：%s", strings.Join(unconfigured, "、"))
	}
	missing := []string{}
	for _, name := range configuredNames {
		if !seen[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("parallel artifact 缺少配置成员：%s", strings.Join(missing, "、"))
	}
	return nil
}

func artifactHasSevereFinding(artifact ParallelArtifact) bool {
	for _, member := range artifact.Members {
		for _, finding := range member.Findings {
			if isCurrentChangeFindingHardBlocking(finding) {
				return true
			}
		}
	}
	return false
}

func artifactHasRequiredFailure(artifact ParallelArtifact) bool {
	for _, member := range artifact.Members {
		if member.Required && !memberStatusSucceeded(member.Status) {
			return true
		}
	}
	return false
}

func artifactHasMemberFailure(artifact ParallelArtifact) bool {
	for _, member := range artifact.Members {
		if !memberStatusSucceeded(member.Status) {
			return true
		}
	}
	return false
}

func memberStatusSucceeded(status string) bool {
	switch normalizeMemberStatus(status) {
	case "success", "passed", "clean", "completed", "ok":
		return true
	default:
		return false
	}
}

func normalizeMemberStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "0", "success", "passed", "clean", "completed", "ok", "pass":
		return "success"
	case "1", "failed", "fail", "failure", "error":
		return "failed"
	default:
		return strings.TrimSpace(status)
	}
}

// parallelArtifactPath returns the run-local artifact path for an enabled helper group.
func parallelArtifactPath(runPath, group string, iteration int) string {
	switch group {
	case "planning_context":
		return filepath.Join(runPath, "parallel-planning-context.json")
	case "implementation_context":
		return filepath.Join(runPath, "parallel-implementation-context.json")
	case "review":
		return filepath.Join(runPath, "parallel-review-"+formatIteration(iteration)+".json")
	case "qa":
		return filepath.Join(runPath, "parallel-qa-"+formatIteration(iteration)+".json")
	default:
		return filepath.Join(runPath, "parallel-"+group+".json")
	}
}

func formatIteration(iteration int) string {
	return strconv.Itoa(iteration)
}

func parseParallelArtifact(path string, data []byte) (ParallelArtifact, error) {
	var artifact ParallelArtifact
	cleaned := bytes.TrimSpace(data)
	cleaned = bytes.TrimPrefix(cleaned, []byte{0xef, 0xbb, 0xbf})
	if len(cleaned) == 0 {
		return ParallelArtifact{}, ReviewArtifactError{Path: path, Code: reviewArtifactParseError, Reason: "artifact is empty"}
	}
	dec := json.NewDecoder(bytes.NewReader(cleaned))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&artifact); err != nil {
		return ParallelArtifact{}, ReviewArtifactError{Path: path, Code: reviewArtifactParseError, Reason: err.Error()}
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return ParallelArtifact{}, ReviewArtifactError{Path: path, Code: reviewArtifactParseError, Reason: "artifact contains trailing content; output must be a single JSON object"}
	}
	return artifact, nil
}
