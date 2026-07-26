// Package app validates durable self-review/self-repair checkpoints.
package app

import (
	"fmt"
	"os"
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
