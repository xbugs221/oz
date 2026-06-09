// Package app validates structured review artifacts produced by Codex.
package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	reviewArtifactParseError      = "parseError"
	reviewArtifactValidationError = "validationError"
)

// Review is the strict JSON contract used by review stages.
type Review struct {
	Summary         string                 `json:"summary"`
	Decision        string                 `json:"decision"`
	Checks          ReviewChecks           `json:"checks"`
	Evidence        []string               `json:"evidence"`
	Findings        []Finding              `json:"findings"`
	WorkflowFailure *ReviewWorkflowFailure `json:"workflow_failure,omitempty"`
}

// ReviewChecks records the review coverage that must be true before clean.
type ReviewChecks struct {
	OzAligned                bool `json:"oz_aligned"`
	TasksVerified            bool `json:"tasks_verified"`
	TestsMeaningful          bool `json:"tests_meaningful"`
	ImplementationScoped     bool `json:"implementation_scoped"`
	RuntimeBehaviorVerified  bool `json:"runtime_behavior_verified"`
	PreviousFindingsResolved bool `json:"previous_findings_resolved"`
}

// Finding describes one reviewer finding.
type Finding struct {
	Title          string `json:"title"`
	Severity       string `json:"severity"`
	Evidence       string `json:"evidence"`
	Recommendation string `json:"recommendation"`
}

// ReviewWorkflowFailure lets reviewer stop futile fix loops through strict JSON.
type ReviewWorkflowFailure struct {
	Failed bool   `json:"failed"`
	Reason string `json:"reason"`
}

// ReviewArtifactError wraps strict review artifact parse/validation failures.
type ReviewArtifactError struct {
	Path   string
	Code   string
	Reason string
}

// Error returns a machine-readable plus human-readable error message.
func (e ReviewArtifactError) Error() string {
	if e.Path == "" {
		return fmt.Sprintf("review artifact %s: %s", e.Code, e.Reason)
	}
	return fmt.Sprintf("review artifact %s (%s): %s", e.Code, e.Path, e.Reason)
}

// ReadReview loads and validates a review JSON file.
func ReadReview(path string) (Review, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Review{}, err
	}
	review, err := parseReviewArtifact(path, data)
	if err != nil {
		if artifactErr, ok := err.(ReviewArtifactError); ok {
			artifactErr.Path = path
			return Review{}, artifactErr
		}
		return Review{}, ReviewArtifactError{Path: path, Code: reviewArtifactParseError, Reason: err.Error()}
	}
	review = normalizeReview(review)
	if err := ValidateReviewForIteration(review, reviewIterationFromPath(path)); err != nil {
		return Review{}, ReviewArtifactError{Path: path, Code: reviewArtifactValidationError, Reason: err.Error()}
	}
	return review, nil
}

// ValidateReview enforces the review JSON schema used by the workflow.
func ValidateReview(review Review) error {
	return ValidateReviewForIteration(review, 1)
}

// ValidateReviewForIteration enforces review checks that depend on review round.
func ValidateReviewForIteration(review Review, iteration int) error {
	if review.Summary == "" {
		return fmt.Errorf("review summary 不能为空")
	}
	if review.Decision != "clean" && review.Decision != "needs_fix" {
		return fmt.Errorf("无效 review decision %q", review.Decision)
	}
	if ReviewDeclaresWorkflowFailure(review) {
		if review.Decision != "needs_fix" {
			return fmt.Errorf("workflow_failure 要求 decision 为 needs_fix")
		}
		if strings.TrimSpace(review.WorkflowFailure.Reason) == "" {
			return fmt.Errorf("workflow_failure.reason 不能为空")
		}
	}
	if review.Decision == "clean" && len(review.Findings) != 0 {
		return fmt.Errorf("clean review 不能包含 findings")
	}
	if review.Decision == "clean" {
		if err := validateCleanReview(review, iteration); err != nil {
			return err
		}
	}
	if review.Decision == "needs_fix" && len(review.Findings) == 0 {
		return fmt.Errorf("needs_fix review 必须包含 findings")
	}
	for i, finding := range review.Findings {
		if finding.Title == "" || finding.Evidence == "" || finding.Recommendation == "" {
			return fmt.Errorf("finding %d 不完整", i)
		}
		if _, ok := normalizeFindingSeverity(finding.Severity); !ok {
			return fmt.Errorf("finding %d 的 severity 无效：%q", i, finding.Severity)
		}
	}
	return nil
}

func normalizeReview(review Review) Review {
	// Normalize model-friendly severity aliases at the file boundary while keeping the
	// internal workflow contract on blocker/major/minor.
	for i := range review.Findings {
		if severity, ok := normalizeFindingSeverity(review.Findings[i].Severity); ok {
			review.Findings[i].Severity = severity
		}
	}
	return review
}

func normalizeFindingSeverity(severity string) (string, bool) {
	// Accept common review vocabulary produced by different agent tools.
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "blocker", "critical":
		return "blocker", true
	case "major", "high", "medium":
		return "major", true
	case "minor", "low", "nit", "info", "informational", "note", "warning":
		return "minor", true
	default:
		return "", false
	}
}

func validateCleanReview(review Review, iteration int) error {
	if !review.Checks.OzAligned ||
		!review.Checks.TasksVerified ||
		!review.Checks.TestsMeaningful ||
		!review.Checks.ImplementationScoped ||
		!review.Checks.RuntimeBehaviorVerified ||
		!review.Checks.PreviousFindingsResolved {
		return fmt.Errorf("clean review 要求所有 checks 为 true")
	}
	for _, evidence := range review.Evidence {
		if strings.TrimSpace(evidence) != "" {
			if hasValidationEvidence(review.Evidence) && hasRuntimeEvidence(review.Evidence) {
				return nil
			}
			return fmt.Errorf("clean review evidence 必须引用验证命令以及截图、trace、QA 或运行时证据")
		}
	}
	return fmt.Errorf("clean review 必须包含 evidence")
}

func hasValidationEvidence(evidence []string) bool {
	for _, item := range evidence {
		text := strings.ToLower(item)
		for _, keyword := range []string{"validation", "test", "go test", "pnpm", "npm", "uv", "pytest", "playwright", "artifact", "门禁"} {
			if strings.Contains(text, keyword) {
				return true
			}
		}
	}
	return false
}

func hasRuntimeEvidence(evidence []string) bool {
	for _, item := range evidence {
		text := strings.ToLower(item)
		for _, keyword := range []string{"screenshot", "trace", "playwright", "browser", "console", "network", "qa", "runtime", "刷新", "截图", "运行时"} {
			if strings.Contains(text, keyword) {
				return true
			}
		}
	}
	return false
}

// parseReviewArtifact parses one review artifact body and enforces strict JSON shape.
func parseReviewArtifact(path string, data []byte) (Review, error) {
	var review Review
	cleaned := bytes.TrimSpace(data)
	cleaned = bytes.TrimPrefix(cleaned, []byte{0xef, 0xbb, 0xbf})
	if len(cleaned) == 0 {
		return Review{}, ReviewArtifactError{Path: path, Code: reviewArtifactParseError, Reason: "artifact is empty"}
	}
	dec := json.NewDecoder(bytes.NewReader(cleaned))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&review); err != nil {
		return Review{}, ReviewArtifactError{Path: path, Code: reviewArtifactParseError, Reason: err.Error()}
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return Review{}, ReviewArtifactError{Path: path, Code: reviewArtifactParseError, Reason: "artifact contains trailing content; output must be a single JSON object"}
	}
	return review, nil
}

func reviewIterationFromPath(path string) int {
	name := strings.TrimSuffix(filepath.Base(path), ".json")
	n, err := strconv.Atoi(strings.TrimPrefix(name, "review-"))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// NeedsFix reports whether a valid review requires a fix stage.
func NeedsFix(review Review) bool {
	return review.Decision == "needs_fix" || len(review.Findings) > 0
}

// ReviewDeclaresWorkflowFailure reports whether reviewer chose to stop the workflow.
func ReviewDeclaresWorkflowFailure(review Review) bool {
	return review.WorkflowFailure != nil && review.WorkflowFailure.Failed
}
