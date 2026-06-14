// Package app validates review artifacts from CLI.
package app

import (
	"fmt"
	"io"
	"strings"
)

type reviewValidationResult struct {
	Path     string `json:"path"`
	Valid    bool   `json:"valid"`
	Decision string `json:"decision,omitempty"`
	Findings int    `json:"findings,omitempty"`
	Error    string `json:"error,omitempty"`
	Code     string `json:"code,omitempty"`
}

func runValidateReviewArtifact(args []string, stdout io.Writer) error {
	if !hasFlag(args, "--artifact") {
		return fmt.Errorf("用法：oz flow validate-review --artifact <artifact-path> [--json]")
	}
	path, err := requireFlagValue(args, "--artifact")
	if err != nil {
		return err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("review artifact 路径不能为空")
	}

	review, err := ReadReview(path)
	if hasFlag(args, "--json") {
		if err != nil {
			result := reviewValidationResult{Path: path, Valid: false}
			if ar, ok := err.(ReviewArtifactError); ok {
				result.Error = ar.Reason
				result.Code = ar.Code
			} else {
				result.Error = err.Error()
				result.Code = "readError"
			}
			_ = writeJSON(stdout, result)
			return err
		}
		result := reviewValidationResult{
			Path:     path,
			Valid:    true,
			Decision: review.Decision,
			Findings: len(review.Findings),
		}
		return writeJSON(stdout, result)
	}

	if err != nil {
		if ar, ok := err.(ReviewArtifactError); ok {
			return fmt.Errorf("%s: %s", ar.Code, ar.Reason)
		}
		return err
	}
	fmt.Fprintf(stdout, "review artifact 合法: %s (decision=%s, findings=%d)\n", path, review.Decision, len(review.Findings))
	return nil
}
